import os
import time
from unittest import mock

import pytest
from cachalot.settings import cachalot_settings
from django.conf import settings
from django.core.management import call_command
from django.db import connection
from django.test import override_settings
from django.test.utils import CaptureQueriesContext

from documents.models import Correspondent
from documents.models import Tag
from paperless import settings as paperless_settings
from paperless.db_cache import custom_get_query_cache_key
from paperless.db_cache import custom_get_table_cache_key
from paperless.db_cache import invalidate_db_cache
from paperless.settings import _parse_cachalot_settings
from paperless.settings import _parse_caches


def _read_tags():
    return list(Tag.objects.values_list("id", flat=True))


def _count_queries(func) -> int:
    with CaptureQueriesContext(connection) as ctx:
        func()
    return len(ctx)


class TestDbCacheSettings:
    def test_cachalot_default_settings(self):
        assert not settings.CACHALOT_ENABLED
        assert "cachalot" not in settings.INSTALLED_APPS

        cachalot = _parse_cachalot_settings()
        caches = _parse_caches()

        assert not cachalot["CACHALOT_ENABLED"]
        assert cachalot["CACHALOT_TIMEOUT"] == 3600
        assert cachalot["CACHALOT_CACHE"] == "read-cache"
        assert (
            cachalot["CACHALOT_QUERY_KEYGEN"]
            == "paperless.db_cache.custom_get_query_cache_key"
        )
        assert (
            cachalot["CACHALOT_TABLE_KEYGEN"]
            == "paperless.db_cache.custom_get_table_cache_key"
        )
        assert cachalot["CACHALOT_FINAL_SQL_CHECK"] is True
        assert caches["read-cache"]["LOCATION"] == "redis://localhost:6379"
        assert caches["read-cache"]["KEY_PREFIX"] == ""
        assert caches["read-cache"]["TIMEOUT"] == 3600

    @mock.patch.dict(
        os.environ,
        {
            "PAPERLESS_DB_READ_CACHE_ENABLED": "true",
            "PAPERLESS_READ_CACHE_REDIS_URL": "redis://localhost:6380/7",
            "PAPERLESS_READ_CACHE_TTL": "7200",
        },
    )
    def test_cachalot_custom_settings(self):
        cachalot = _parse_cachalot_settings()
        caches = _parse_caches()

        assert cachalot["CACHALOT_ENABLED"]
        assert cachalot["CACHALOT_TIMEOUT"] == 7200
        assert cachalot["CACHALOT_REDIS_URL"] == "redis://localhost:6380/7"
        assert caches["read-cache"]["LOCATION"] == "redis://localhost:6380/7"
        assert caches["read-cache"]["TIMEOUT"] == 7200
        assert caches["default"]["LOCATION"] == "redis://localhost:6379"

    @mock.patch.dict(
        os.environ,
        {"PAPERLESS_READ_CACHE_REDIS_URL": "unix:///run/redis.sock?db=3"},
    )
    def test_cachalot_redis_socket_url(self):
        cachalot = _parse_cachalot_settings()
        assert cachalot["CACHALOT_REDIS_URL"] == "unix:///run/redis.sock?db=3"

    @pytest.mark.parametrize(
        ("env_var_ttl", "expected_cachalot_timeout"),
        [
            # Zero, negative or invalid values are ignored
            ("0", 3600),
            ("-1", 3600),
            ("-500000", 3600),
            ("not a number", 3600),
            ("1.5", 3600),
            ("", 3600),
            # Positive values are used, up to one year
            ("1", 1),
            ("7524", 7524),
            ("31536000", 31536000),
            ("99999999999999", 31536000),
        ],
    )
    def test_cachalot_ttl_parsing(self, env_var_ttl, expected_cachalot_timeout):
        with mock.patch.dict(os.environ, {"PAPERLESS_READ_CACHE_TTL": env_var_ttl}):
            ttl = _parse_cachalot_settings()["CACHALOT_TIMEOUT"]
        assert ttl == expected_cachalot_timeout

    def test_all_caches_have_same_custom_prefix_from_module(self, monkeypatch):
        monkeypatch.delenv("PAPERLESS_REDIS_PREFIX", raising=False)
        monkeypatch.setattr(paperless_settings, "_REDIS_KEY_PREFIX", "custom_prefix")
        caches = _parse_caches()
        assert caches["default"]["KEY_PREFIX"] == "custom_prefix"
        assert caches["read-cache"]["KEY_PREFIX"] == "custom_prefix"

    def test_all_caches_have_same_custom_prefix_from_env(self, monkeypatch):
        monkeypatch.setenv("PAPERLESS_REDIS_PREFIX", "env_prefix")
        caches = _parse_caches()
        assert caches["default"]["KEY_PREFIX"] == "env_prefix"
        assert caches["read-cache"]["KEY_PREFIX"] == "env_prefix"

    def test_read_cache_backend_follows_cache_backend(self, monkeypatch):
        monkeypatch.delenv("PAPERLESS_CACHE_BACKEND", raising=False)
        caches = _parse_caches()
        assert (
            caches["read-cache"]["BACKEND"]
            == "django.core.cache.backends.redis.RedisCache"
        )

        monkeypatch.setenv(
            "PAPERLESS_CACHE_BACKEND",
            "django.core.cache.backends.locmem.LocMemCache",
        )
        caches = _parse_caches()
        assert (
            caches["read-cache"]["BACKEND"]
            == "django.core.cache.backends.locmem.LocMemCache"
        )

    def test_custom_cache_keys_are_namespaced(self):
        assert custom_get_table_cache_key("default", "documents_tag").startswith(
            "cachalot:table:",
        )
        compiler = Tag.objects.all().query.get_compiler(using="default")
        assert custom_get_query_cache_key(compiler).startswith("cachalot:query:")


@pytest.mark.django_db(transaction=True)
class TestDbCache:
    @pytest.fixture(autouse=True)
    def _reset_cache(self):
        cachalot_settings.reload()
        invalidate_db_cache()
        yield
        invalidate_db_cache()

    def test_cache_is_disabled_by_default(self):
        assert not cachalot_settings.CACHALOT_ENABLED
        for _ in range(3):
            assert _count_queries(_read_tags)

    @override_settings(CACHALOT_ENABLED=True, CACHALOT_TIMEOUT=1)
    def test_cache_hit_when_enabled_and_expires_after_ttl(self):
        cachalot_settings.reload()
        assert cachalot_settings.CACHALOT_ENABLED
        assert cachalot_settings.CACHALOT_TIMEOUT == 1

        assert _count_queries(_read_tags)
        assert not _count_queries(_read_tags)

        time.sleep(1.1)

        assert _count_queries(_read_tags)
        assert not _count_queries(_read_tags)

    @override_settings(CACHALOT_ENABLED=True)
    def test_cache_is_invalidated_on_write(self):
        cachalot_settings.reload()

        assert _read_tags() == []
        assert not _count_queries(_read_tags)

        tag = Tag.objects.create(name="tag")

        assert _count_queries(_read_tags)
        assert _read_tags() == [tag.id]

    @override_settings(CACHALOT_ENABLED=True)
    def test_invalidate_command(self):
        cachalot_settings.reload()

        _read_tags()
        assert not _count_queries(_read_tags)

        call_command("invalidate_cachalot", verbosity=0)

        assert _count_queries(_read_tags)

    @override_settings(CACHALOT_ENABLED=True)
    def test_invalidate_command_for_model(self):
        cachalot_settings.reload()

        def read_correspondents():
            return list(Correspondent.objects.values_list("id", flat=True))

        _read_tags()
        read_correspondents()

        call_command("invalidate_cachalot", "documents.Tag", verbosity=0)

        assert _count_queries(_read_tags)
        assert not _count_queries(read_correspondents)

        call_command("invalidate_cachalot", "documents", verbosity=0)

        assert _count_queries(read_correspondents)

    def test_settings_change_is_applied_to_cachalot(self):
        assert not cachalot_settings.CACHALOT_ENABLED

        with override_settings(CACHALOT_ENABLED=True, CACHALOT_TIMEOUT=42):
            assert cachalot_settings.CACHALOT_ENABLED
            assert cachalot_settings.CACHALOT_TIMEOUT == 42
            _read_tags()
            assert not _count_queries(_read_tags)

        assert not cachalot_settings.CACHALOT_ENABLED
        assert cachalot_settings.CACHALOT_TIMEOUT == settings.CACHALOT_TIMEOUT
        assert _count_queries(_read_tags)
