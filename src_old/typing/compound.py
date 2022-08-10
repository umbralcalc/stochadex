from dataclasses import Field, dataclass, asdict, fields
from typing import Any, Dict, List, Optional, Tuple, Union
import pandas as pd
import streamlit as st
import matplotlib.pyplot as plt
import seaborn as sns
import datetime as dt

from src.typing.simple import *
import src.anglersim.simio.simio_pb2 as simio


class DataframeableData:
    """base class for data which can turn into dataframes"""

    def to_dict(self) -> dict:
        return asdict(self)

    def to_df(self) -> pd.DataFrame:
        return pd.DataFrame.from_dict(self.to_dict())


@dataclass
class Species(DataframeableData):
    id: List[str]
    name: List[str]


@dataclass
class Surveys(DataframeableData):
    id: List[str]
    method: List[str]
    strategy: List[str]
    length: List[float]
    width: List[float]
    area: List[float]
    species_selective: List[bool]  # important one here!
    third_party: List[bool]


@dataclass
class Sites(DataframeableData):
    id: List[str]
    name: List[str]
    location_name: List[str]
    region: List[str]
    country: List[str]
    geo_waterbody: List[str]


@dataclass
class Counts(DataframeableData):
    by_run: Dict[Run, List[Union[int, Any]]]
    date: List[dt.datetime]

    def to_dict(self) -> dict:
        dat = {str(k): v for k, v in self.by_run.items()}
        dat["date"] = self.date
        return dat


@dataclass
class PlottableData(DataframeableData):
    """base class to retrieve plotting data"""

    def to_dict(self) -> dict:
        cls_fields: Tuple[Field, ...] = fields(self.__class__)
        overall_dict = {}
        for field in cls_fields:
            new_dict = getattr(getattr(self, field.name), "to_dict")()
            new_dict = {key: new_dict[key] for key in new_dict}
            overall_dict.update(
                {field.name + '.' + key: new_dict[key] for key in new_dict}
            )
        return overall_dict


@dataclass
class PlotConfig:
    """base class for plot configs"""

    data: PlottableData
    fig: Optional[plt.figure] = None

    def __post_init__(self):
        sns.set()
        self.fig = plt.figure()
        self._df = self.data.to_df()

    def set_user_settings(self):
        """call streamlit settings routine
        and set config attributes"""

    def create_plot(self):
        """create the actual plot"""

    def show(self):
        st.pyplot(self.fig)


@dataclass
class FWFishCounts(PlottableData):
    sites: Sites
    species: Species
    surveys: Surveys
    counts: Counts


@dataclass
class AppConfig:
    mode: AppModes
    fish_count_data: FWFishCounts


@dataclass
class FishPop:
    name: str
    count: int
    birth_rate: float
    dens_dep_power: float
    death_rate: float
    pred_rate: float
    pred_birth_inc_rate: float
    fishing_rate: float
    pred_matrix_row: List[float]
    prey_matrix_row: List[float]


@dataclass
class APIConfig:
    run_time: float
    time_scale: float
    num_reals: int
    num_species: int
    seed: int
    fish_pops: List[FishPop]

    def to_protobuf(self) -> simio.AnglersimInput:
        message = simio.AnglersimInput()
        message.runTime = self.run_time
        message.timeScale = self.time_scale
        message.numReals = self.num_reals
        message.numSpecies = self.num_species
        message.seed = self.seed
        for i in range(self.num_species):
            fish_pop = message.fishPops.add()
            fish_pop.initCount = self.fish_pops[i].count
            fish_pop.densDepPower = self.fish_pops[i].dens_dep_power
            fish_pop.birthRate = self.fish_pops[i].birth_rate
            fish_pop.deathRate = self.fish_pops[i].death_rate
            fish_pop.predationRate = self.fish_pops[i].pred_rate
            fish_pop.predatorBirthIncRate = self.fish_pops[i].pred_birth_inc_rate
            fish_pop.fishingRate = self.fish_pops[i].fishing_rate
            fish_pop.predatorMatrixRow = self.fish_pops[i].pred_matrix_row
            fish_pop.preyMatrixRow = self.fish_pops[i].prey_matrix_row