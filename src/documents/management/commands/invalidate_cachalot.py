from django.conf import settings
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = "Invalidate the database read cache (cachalot)"

    def handle(self, *args, **options):
        if not settings.CACHALOT_ENABLED:
            self.stdout.write("Database read cache is not enabled.")
            return
        from cachalot.api import invalidate

        invalidate(cachealias="read-cache")
        self.stdout.write("Database read cache cleared.")
