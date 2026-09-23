from django.core.management.base import BaseCommand

from paperless.db_cache import invalidate_db_cache


class Command(BaseCommand):
    help = "Clear the optional database read cache."

    def handle(self, *args, **options):
        invalidate_db_cache()
        self.stdout.write(self.style.SUCCESS("Database read cache cleared."))
