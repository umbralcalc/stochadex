import pandas as pd
import logging

from src.data.utils import UNZIPPED_FILES
from src.data.database import get_unzipped_files
from src.typing.simple import *
from src.typing.compound import *


def retrieve_fish_counts() -> FWFishCounts:
    # Retrieve the fish counts data
    get_unzipped_files("NFPD_FWfish_counts")
    logging.info("retrieving fish counts dataset")
    df = pd.read_csv("data/" + UNZIPPED_FILES["NFPD_FWfish_counts"][0])
    by_run = {
        Run(str(i)): list(df["RUN" + str(i)]) 
        for i in range(1, 7)
    }
    return FWFishCounts(
        sites=Sites(
            id=list(df.SITE_ID),
            name=list(df.SITE_NAME),
            location_name=list(df.LOCATION_NAME),
            region=list(df.REGION),
            country=list(df.COUNTRY),
            geo_waterbody=list(df.GEO_WATERBODY),
        ),
        species=Species(id=list(df.SPECIES_ID), name=list(df.SPECIES_NAME)),
        surveys=Surveys(
            id=list(df.SURVEY_ID),
            method=list(df.SURVEY_METHOD),
            strategy=list(df.SURVEY_STRATEGY),
            length=list(df.SURVEY_LENGTH),
            width=list(df.SURVEY_WIDTH),
            area=list(df.SURVEY_AREA),
            species_selective=list(df.IS_SPECIES_SELECTIVE),
            third_party=list(df.IS_THIRD_PARTY),
        ),
        counts=Counts(by_run=by_run, date=list(pd.to_datetime(df.EVENT_DATE))),
    )