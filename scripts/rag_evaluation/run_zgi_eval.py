#!/usr/bin/env python3
"""Run the ZGI-only stage of the decoupled RAG comparison workflow."""

from run_ragas_eval import main


if __name__ == "__main__":
    raise SystemExit(main(output_platform="zgi"))
