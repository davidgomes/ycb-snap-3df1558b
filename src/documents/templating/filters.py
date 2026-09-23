from datetime import date
from datetime import datetime

from babel import Locale
from babel import UnknownLocaleError
from babel import dates
from django.utils.dateparse import parse_date


def localize_date(value: date | datetime, format: str, locale: str) -> str:
    """
    Formats a date or datetime into a localized string using Babel.

    Args:
        value: The date or datetime to format. A timezone-aware datetime is
            rendered in its own timezone; a naive one is treated as UTC.
        format: Either one of Babel's preset formats ("short", "medium",
            "long", "full") or a CLDR/Unicode pattern such as "dd.MM.yyyy".
        locale: A locale identifier such as "en_US" or "de_DE".

    Raises:
        TypeError: If value is not a date or datetime.
        ValueError: If locale is not a valid or known locale identifier.
    """
    try:
        parsed_locale = Locale.parse(locale)
    except (ValueError, TypeError, UnknownLocaleError) as e:
        raise ValueError(f"Invalid locale identifier: {locale}") from e

    if isinstance(value, datetime):
        return dates.format_datetime(value, format=format, locale=parsed_locale)
    if isinstance(value, date):
        return dates.format_date(value, format=format, locale=parsed_locale)
    raise TypeError(
        f"Unsupported type {type(value)} for localize_date, expected a date or datetime",
    )


def format_datetime(value: str | datetime, format: str) -> str:
    if isinstance(value, str):
        value = parse_date(value)
    return value.strftime(format=format)


def get_cf_value(
    custom_field_data: dict[str, dict[str, str]],
    name: str,
    default: str | None = None,
) -> str | None:
    if name in custom_field_data and custom_field_data[name]["value"] is not None:
        return custom_field_data[name]["value"]
    elif default is not None:
        return default
    return None
