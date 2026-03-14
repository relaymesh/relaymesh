import importlib
import os
from pathlib import Path


def read_version() -> str:
    raw = (os.getenv("RELAYMESH_PY_VERSION") or "").strip()
    if raw.startswith("v"):
        raw = raw[1:]
    return raw or "0.0.19"


def read_readme() -> str:
    path = Path(__file__).with_name("README.md")
    return path.read_text(encoding="utf-8")


setuptools = importlib.import_module("setuptools")
find_packages = setuptools.find_packages
setup = setuptools.setup

setup(
    name="relaymesh",
    version=read_version(),
    description="Relaymesh worker SDK",
    long_description=read_readme(),
    long_description_content_type="text/markdown",
    python_requires=">=3.9",
    packages=find_packages(where=".", include=["relaymesh*", "cloud*", "buf*"]),
    install_requires=[
        "protobuf>=6.33.5",
        "relaybus-amqp",
        "relaybus-kafka",
        "relaybus-nats",
    ],
)
