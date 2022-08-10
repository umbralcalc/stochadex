import subprocess
import anglersim.simio.simio_pb2 as simio
from typing.compound import APIConfig


class AnglersimAPI:
    def __init__(self, config: APIConfig):
        self.config = config
        self.sim_input = config.to_protobuf()
        self.sim_output = simio.AnglersimOutput()

    def run(self):
        # write protobuf message to input files
        f = open("anglersim/input/anglersim.data", "wb")
        f.write(self.sim_input.SerializeToString())
        f.close()

        # call anglersim and run it
        subprocess.Popen(["go", "run", "."], cwd="anglersim/")

        # read in protobuf message output
        f = open("anglersim/output/anglersim.data", "rb")
        self.sim_output.ParseFromString(f.read())
        f.close()