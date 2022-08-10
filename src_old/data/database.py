"""
Ecological database tools for data obtained from various
open sources. Here are some generally useful links to
datasets and APIs:
https://environment.data.gov.uk/apiportal
https://hub.jncc.gov.uk/assets/52b4e00d-798e-4fbe-a6ca-2c5735ddf049
https://www.cefas.co.uk/data-and-publications/cefas-data-portal-apis/

"""

import os
import subprocess
from pathlib import Path
from src.data.utils import LINKS, UNZIPPED_FILES


def get_unzipped_files(name: str):
    # if the folder doesn't even exist then make it
    pth = Path("data")
    pth.mkdir(parents=True, exist_ok=True)

    if not os.path.exists("data/" + name + ".zip"):
        bashCommand = "wget " + LINKS[name]
        bashCommand += " -O data/" + name + ".zip"
        process = subprocess.Popen(bashCommand.split())
        output, error = process.communicate()

    # Unzip the relevant files
    bashCommand = "unzip -o " + name + ".zip"
    for uzf in UNZIPPED_FILES[name]:
        bashCommand += " " + uzf
    process = subprocess.Popen(
        bashCommand.split(),
        cwd="data/",
    )
    output, error = process.communicate()
