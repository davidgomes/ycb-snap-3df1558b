from django.apps import AppConfig
from django.utils.translation import gettext_lazy as _

from paperless.signals import handle_failed_login
from paperless.signals import handle_social_account_updated


class PaperlessConfig(AppConfig):
    name = "paperless"

    verbose_name = _("Paperless")

    def ready(self):
        from django.contrib.auth.signals import user_login_failed

        user_login_failed.connect(handle_failed_login)

        from allauth.socialaccount.signals import social_account_updated

        social_account_updated.connect(handle_social_account_updated)

        from django.core.signals import setting_changed

        from paperless.db_cache import handle_cachalot_setting_changed

        setting_changed.connect(handle_cachalot_setting_changed)

        AppConfig.ready(self)
