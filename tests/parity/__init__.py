"""Differential parity harness package (card [Go 03]).

Re-export the core module so that ``import parity`` resolves to the same
API regardless of whether ``tests/`` or ``tests/parity/`` is on sys.path:
when ``tests/`` is importable (e.g. ``python3 -m unittest tests.…``), the
package name shadows ``parity.py``; the re-export keeps both routes equal.
"""
from .parity import *  # noqa: F401,F403
