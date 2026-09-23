from django.apps import apps
from django.core.management.base import BaseCommand

from paperless.db_cache import invalidate_db_cache


class Command(BaseCommand):
    # Shadows the django-cachalot command of the same name, so the read cache
    # can be invalidated even while it is disabled and cachalot isn't installed
    help = (
        "Invalidates the database read cache, for all models or only for "
        "the given apps or models"
    )

    def add_arguments(self, parser):
        parser.add_argument(
            "labels",
            nargs="*",
            metavar="app_label[.model_name]",
            help="Only invalidate the cache of these apps or models",
        )

    def handle(self, *args, **options):
        models = []
        for label in options["labels"]:
            if "." in label:
                models.append(apps.get_model(label))
            else:
                models.extend(apps.get_app_config(label).get_models())

        if options["labels"] and not models:
            self.stdout.write("No models to invalidate.")
            return

        invalidate_db_cache(*models)

        if options["verbosity"] > 0:
            self.stdout.write(self.style.SUCCESS("Database read cache invalidated."))
