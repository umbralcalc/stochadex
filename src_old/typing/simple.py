from enum import Enum


class AppModes(Enum):
    data_plotter = "data plotter"


class Run(str):
    def __new__(cls, *args, **kwargs):
        return super(Run, cls).__new__(cls, *args, **kwargs)
