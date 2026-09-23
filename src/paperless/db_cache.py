from cachalot.api import invalidate as cachalot_invalidate
from cachalot.utils import get_query_cache_key
from cachalot.utils import get_table_cache_key
from django.conf import settings
from django.db.models.sql.compiler import SQLCompiler

# The read cache may share its Redis database with the Django cache and
# Celery, so the keys are made recognizable
PREFIX = "pngx_cachalot_"


def custom_get_query_cache_key(compiler: SQLCompiler) -> str:
    return PREFIX + get_query_cache_key(compiler)


def custom_get_table_cache_key(db_alias: str, table: str) -> str:
    return PREFIX + get_table_cache_key(db_alias, table)


def invalidate_db_cache() -> None:
    """
    Invalidates every cached database query, for all tables and databases.
    """
    cachalot_invalidate(cache_alias=settings.CACHALOT_CACHE)
