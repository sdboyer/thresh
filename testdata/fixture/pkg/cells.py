"""A Python registry and a declared set of functions."""

CELLS = {
    "decode": "the decode cell",
    "prefill": "the prefill cell",
}

ROUTES = {
    ("wgmma", "forward"): "the port",
}


def cell_decode():
    return 1


def cell_prefill():
    return 2
