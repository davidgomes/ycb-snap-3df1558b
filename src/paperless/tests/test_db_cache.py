import time
from collections.abc import Callable
from collections.abc import Generator

import pytest
from cachalot.settings import cachalot_settings
from django.db import connection
from django.test.utils import CaptureQueriesContext
from pytest_django.fixtures import SettingsWrapper

from documents.models import Tag
from paperless.db_cache import PREFIX
from paperless.db_cache import custom_get_query_cache_key
from paperless.db_cache import custom_get_table_cache_key
from paperless.db_cache import invalidate_db_cache
from paperless.settings import _parse_cachalot_settings
from paperless.settings import _parse_caches


@pytest.fixture
def clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    for key in (
        "PAPERLESS_DB_READ_CACHE_ENABLED",
        "PAPERLESS_READ_CACHE_TTL",
        "PAPERLESS_READ_CACHE_REDIS_URL",
        "PAPERLESS_REDIS",
        "PAPERLESS_REDIS_PREFIX",
    ):
        monkeypatch.delenv(key, raising=False)


@pytest.mark.usefixtures("clean_env")
class TestDbCacheSettings:
    def test_default_settings(self) -> None:
        """
        GIVEN:
            - No database read cache environment variables are set
        WHEN:
            - The read cache settings are parsed
        THEN:
            - The read cache is disabled, with a TTL of one hour
            - The read cache uses the default Redis, without key prefix
        """
        cachalot = _parse_cachalot_settings()
        caches = _parse_caches()

        assert cachalot["CACHALOT_ENABLED"] is False
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
        assert caches["read-cache"]["BACKEND"] == caches["default"]["BACKEND"]

    def test_custom_settings(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """
        GIVEN:
            - The database read cache is enabled with a custom TTL and Redis
        WHEN:
            - The read cache settings are parsed
        THEN:
            - The read cache is enabled with the custom TTL and Redis
            - The default cache still uses the default Redis
        """
        monkeypatch.setenv("PAPERLESS_DB_READ_CACHE_ENABLED", "true")
        monkeypatch.setenv("PAPERLESS_READ_CACHE_TTL", "7200")
        monkeypatch.setenv("PAPERLESS_READ_CACHE_REDIS_URL", "redis://cache:6380/7")

        cachalot = _parse_cachalot_settings()
        caches = _parse_caches()

        assert cachalot["CACHALOT_ENABLED"] is True
        assert cachalot["CACHALOT_TIMEOUT"] == 7200
        assert cachalot["CACHALOT_CACHE"] == "read-cache"
        assert caches["read-cache"]["LOCATION"] == "redis://cache:6380/7"
        assert caches["default"]["LOCATION"] == "redis://localhost:6379"

    @pytest.mark.parametrize(
        ("env_ttl", "expected_ttl"),
        [
            pytest.param("0", 3600, id="zero-ignored"),
            pytest.param("-1", 3600, id="negative-ignored"),
            pytest.param("-500000", 3600, id="large-negative-ignored"),
            pytest.param("an hour", 3600, id="not-a-number-ignored"),
            pytest.param("1", 1, id="one-second"),
            pytest.param("7524", 7524, id="custom"),
            pytest.param("31536000", 31536000, id="one-year"),
            pytest.param("99999999999999", 31536000, id="capped-to-one-year"),
        ],
    )
    def test_ttl_parsing(
        self,
        monkeypatch: pytest.MonkeyPatch,
        env_ttl: str,
        expected_ttl: int,
    ) -> None:
        monkeypatch.setenv("PAPERLESS_READ_CACHE_TTL", env_ttl)

        assert _parse_cachalot_settings()["CACHALOT_TIMEOUT"] == expected_ttl

    @pytest.mark.parametrize(
        ("paperless_redis", "read_cache_redis", "expected_location"),
        [
            pytest.param(None, None, "redis://localhost:6379", id="default"),
            pytest.param(
                "redis://broker:6379/1",
                None,
                "redis://broker:6379/1",
                id="falls-back-to-paperless-redis",
            ),
            pytest.param(
                "redis://broker:6379/1",
                "redis://cache:6380/2",
                "redis://cache:6380/2",
                id="dedicated-redis",
            ),
            pytest.param(
                None,
                "redis+socket:///run/redis.sock?virtual_host=3",
                "unix:///run/redis.sock?db=3",
                id="celery-socket-converted",
            ),
        ],
    )
    def test_read_cache_location(
        self,
        monkeypatch: pytest.MonkeyPatch,
        paperless_redis: str | None,
        read_cache_redis: str | None,
        expected_location: str,
    ) -> None:
        if paperless_redis is not None:
            monkeypatch.setenv("PAPERLESS_REDIS", paperless_redis)
        if read_cache_redis is not None:
            monkeypatch.setenv("PAPERLESS_READ_CACHE_REDIS_URL", read_cache_redis)

        assert _parse_caches()["read-cache"]["LOCATION"] == expected_location

    def test_all_caches_have_same_custom_prefix(
        self,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        """
        GIVEN:
            - A custom Redis key prefix
        WHEN:
            - The caches are parsed
        THEN:
            - The prefix is used by both the default and the read cache
        """
        monkeypatch.setenv("PAPERLESS_REDIS_PREFIX", "test_a_custom_key_prefix")

        caches = _parse_caches()

        assert caches["default"]["KEY_PREFIX"] == "test_a_custom_key_prefix"
        assert caches["read-cache"]["KEY_PREFIX"] == "test_a_custom_key_prefix"


def test_cache_keys_are_prefixed() -> None:
    table_key = custom_get_table_cache_key("default", "documents_tag")

    assert table_key.startswith(PREFIX)
    assert table_key != custom_get_table_cache_key("default", "documents_document")

    compiler = Tag.objects.filter(name="a").query.get_compiler("default")
    assert custom_get_query_cache_key(compiler).startswith(PREFIX)


def _tag_names() -> list[str]:
    return list(Tag.objects.order_by("name").values_list("name", flat=True))


def _count_queries(func: Callable[[], object]) -> int:
    with CaptureQueriesContext(connection) as context:
        func()
    return len(context.captured_queries)


@pytest.mark.django_db
def test_cache_disabled_by_default(settings: SettingsWrapper) -> None:
    """
    GIVEN:
        - Default settings
    WHEN:
        - The same query is run multiple times
    THEN:
        - cachalot is not installed and every query hits the database
    """
    assert settings.CACHALOT_ENABLED is False
    assert "cachalot" not in settings.INSTALLED_APPS

    for _ in range(3):
        assert _count_queries(_tag_names) == 1


@pytest.fixture
def enable_read_cache(
    settings: SettingsWrapper,
) -> Generator[Callable[..., None], None, None]:
    def enable(timeout: int = 60) -> None:
        settings.CACHALOT_ENABLED = True
        settings.CACHALOT_TIMEOUT = timeout
        cachalot_settings.reload()

    yield enable

    if cachalot_settings.patched:
        invalidate_db_cache()
        cachalot_settings.unload()


@pytest.mark.django_db(transaction=True)
class TestDbCacheEnabled:
    def test_repeated_query_is_cached(self, enable_read_cache) -> None:
        Tag.objects.create(name="a")
        enable_read_cache()

        assert _count_queries(_tag_names) == 1
        assert _count_queries(_tag_names) == 0
        assert _tag_names() == ["a"]

    def test_cache_expires_after_ttl(self, enable_read_cache) -> None:
        enable_read_cache(timeout=1)

        assert _count_queries(_tag_names) == 1
        assert _count_queries(_tag_names) == 0

        time.sleep(1.5)

        assert _count_queries(_tag_names) == 1
        assert _count_queries(_tag_names) == 0

    def test_write_invalidates_cache(self, enable_read_cache) -> None:
        enable_read_cache()
        Tag.objects.create(name="a")
        assert _tag_names() == ["a"]

        Tag.objects.create(name="b")

        assert _count_queries(_tag_names) == 1
        assert _tag_names() == ["a", "b"]

    def test_manual_invalidation(self, enable_read_cache) -> None:
        enable_read_cache()

        assert _count_queries(_tag_names) == 1
        assert _count_queries(_tag_names) == 0

        invalidate_db_cache()

        assert _count_queries(_tag_names) == 1
