"""
Helpers for the optional database read cache, provided by django-cachalot.

django-cachalot is imported lazily, so it is never loaded unless the read cache
is enabled or explicitly used (e.g. to invalidate it).
"""

from __future__ import annotations

import sys
from typing import TYPE_CHECKING

from django.conf import settings

if TYPE_CHECKING:
    from django.db.models.sql.compiler import SQLCompiler

CACHALOT_KEY_PREFIX = "cachalot:"


def custom_get_query_cache_key(compiler: SQLCompiler) -> str:
    """
    Namespaces the default cachalot query key, so read cache keys can be told
    apart from other keys stored in the same Redis database
    """
    from cachalot.utils import get_query_cache_key

    return f"{CACHALOT_KEY_PREFIX}query:{get_query_cache_key(compiler)}"


def custom_get_table_cache_key(db_alias: str, table: str) -> str:
    """
    Namespaces the default cachalot table key, so read cache keys can be told
    apart from other keys stored in the same Redis database
    """
    from cachalot.utils import get_table_cache_key

    return f"{CACHALOT_KEY_PREFIX}table:{get_table_cache_key(db_alias, table)}"


def load_cachalot_settings() -> None:
    """
    Loads the cachalot settings from the Django settings.  The first load also
    patches the ORM, as the cachalot app does when it is installed.
    """
    from cachalot.settings import cachalot_settings

    cachalot_settings.load()


def invalidate_db_cache(*tables_or_models) -> None:
    """
    Invalidates the database read cache, for the given tables or models or for
    every table if none are given.  Works even when the read cache is disabled,
    so stale entries cannot be served once it is enabled again.
    """
    from cachalot.api import invalidate
    from cachalot.settings import cachalot_settings

    if not cachalot_settings.patched:
        load_cachalot_settings()

    invalidate(*tables_or_models, cache_alias=settings.CACHALOT_CACHE)


def handle_cachalot_setting_changed(*, setting: str, **kwargs) -> None:
    """
    Keeps cachalot in sync with the Django settings when they change at
    runtime (e.g. override_settings), as cachalot only reads them on load
    """
    if not setting.startswith("CACHALOT_"):
        return
    cachalot_settings_module = sys.modules.get("cachalot.settings")
    if cachalot_settings_module is None:
        return
    cachalot_settings = cachalot_settings_module.cachalot_settings
    # Only refresh the values: re-patching now could break an open atomic block
    if cachalot_settings.patched:
        cachalot_settings.load()
