#!/bin/bash
python3.8 -m virtualenv venv
source venv/bin/activate
${PWD}/venv/bin/python -m pip install --upgrade pip
pip install -r requirements.txt
pre-commit install
