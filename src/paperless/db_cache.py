from __future__ import annotations

from typing import TYPE_CHECKING

from cachalot.api import invalidate as cachalot_invalidate
from cachalot.utils import get_query_cache_key
from cachalot.utils import get_table_cache_key

if TYPE_CHECKING:
    from django.db.models.sql.compiler import SQLCompiler

PREFIX = "pngx_cachalot_"


def custom_get_query_cache_key(compiler: SQLCompiler) -> str:
    return PREFIX + get_query_cache_key(compiler)


def custom_get_table_cache_key(db_alias: str, table: str) -> str:
    return PREFIX + get_table_cache_key(db_alias, table)


def invalidate_db_cache() -> None:
    cachalot_invalidate(cache_alias="read-cache")
