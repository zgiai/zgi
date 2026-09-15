import importlib
import json


MODULES = [
    "numpy",
    "pandas",
    "scipy",
    "statsmodels",
    "sklearn",
    "lifelines",
    "linearmodels",
    "xgboost",
    "lightgbm",
    "gseapy",
    "rdkit",
    "neurokit2",
    "scanpy",
    "anndata",
    "matplotlib",
    "seaborn",
    "openpyxl",
    "xlrd",
    "rpy2.robjects",
    "causallearn",
    "lingam",
]


def main() -> int:
    missing = []
    versions = {}
    for name in MODULES:
        try:
            module = importlib.import_module(name)
            versions[name] = getattr(module, "__version__", "unknown")
        except Exception as exc:
            missing.append({"module": name, "error": type(exc).__name__})

    if not missing:
        import numpy as np
        import pandas as pd
        import statsmodels.api as sm
        from lifelines import KaplanMeierFitter
        from linearmodels.iv import IV2SLS
        from lightgbm import LGBMClassifier
        import gseapy as gp
        from rdkit import Chem
        from rdkit.Chem import AllChem, Descriptors
        import neurokit2 as nk
        import anndata as ad
        import scanpy as sc
        import matplotlib.pyplot as plt
        from xgboost import XGBClassifier
        from scipy import stats
        from sklearn.metrics import roc_auc_score
        import rpy2.robjects as ro
        from rpy2.robjects.packages import importr
        from causallearn.search.ConstraintBased.PC import pc
        import lingam

        frame = pd.DataFrame({"x": [1.0, 2.0, 3.0, 4.0], "y": [2.0, 4.1, 5.9, 8.2]})
        fit = sm.OLS(frame["y"], sm.add_constant(frame[["x"]])).fit()
        assert np.isfinite(fit.params).all()
        assert np.isfinite(stats.pearsonr(frame["x"], frame["y"]).statistic)
        assert roc_auc_score([0, 0, 1, 1], [0.1, 0.4, 0.6, 0.9]) == 1.0
        KaplanMeierFitter().fit([1, 2, 3, 4], event_observed=[1, 0, 1, 1])
        iv_frame = pd.DataFrame({"y": [1.0, 2.2, 2.8, 4.1], "x": [0.5, 1.1, 1.4, 2.0], "z": [0.2, 0.8, 1.0, 1.7]})
        IV2SLS(iv_frame[["y"]], pd.DataFrame({"const": np.ones(4)}), iv_frame[["x"]], iv_frame[["z"]]).fit()
        XGBClassifier(n_estimators=2, n_jobs=1, eval_metric="logloss").fit([[0.0], [1.0], [2.0], [3.0]], [0, 0, 1, 1])
        LGBMClassifier(n_estimators=2, n_jobs=1, verbose=-1).fit([[0.0], [1.0], [2.0], [3.0]], [0, 0, 1, 1])
        enrichment_input = pd.DataFrame({"S1": np.arange(20, dtype=float), "S2": np.arange(20, 0, -1, dtype=float)}, index=[f"G{i}" for i in range(20)])
        enrichment = gp.ssgsea(data=enrichment_input, gene_sets={"PATH": [f"G{i}" for i in range(10)]}, outdir=None, min_size=2, max_size=20, threads=1, no_plot=True, verbose=False)
        assert not enrichment.res2d.empty
        molecule = Chem.MolFromSmiles("CCO")
        assert molecule is not None and Descriptors.MolWt(molecule) > 40
        assert AllChem.GetMorganFingerprintAsBitVect(molecule, 2, nBits=64).GetNumBits() == 64
        assert len(nk.ecg_simulate(duration=2, sampling_rate=100, heart_rate=70)) == 200
        tiny = ad.AnnData(np.asarray([[1.0, 0.0, 2.0], [0.0, 3.0, 1.0]], dtype=np.float32))
        sc.pp.normalize_total(tiny, target_sum=100.0)
        assert tiny.shape == (2, 3) and np.isfinite(tiny.X).all()
        figure, axis = plt.subplots(figsize=(2, 2))
        axis.plot([0.0, 1.0], [0.0, 1.0])
        figure.canvas.draw()
        plt.close(figure)
        assert float(ro.r("sum(c(1, 2, 3))")[0]) == 6.0
        for package in ("edgeR", "DESeq2", "sva", "MendelianRandomization"):
            importr(package)
        causal_x = np.linspace(-1.0, 1.0, 80)
        causal_data = np.column_stack([causal_x, causal_x + np.sin(np.arange(80)) * 0.1, np.cos(np.arange(80) * 0.31)])
        assert pc(causal_data, alpha=0.05, indep_test="fisherz", show_progress=False).G.graph.shape == (3, 3)
        lingam_model = lingam.DirectLiNGAM(random_state=42).fit(causal_data)
        assert lingam_model.adjacency_matrix_.shape == (3, 3)

    print(json.dumps({"profile": "scientific-data", "versions": versions, "missing": missing, "ok": not missing}))
    return 1 if missing else 0


if __name__ == "__main__":
    raise SystemExit(main())
