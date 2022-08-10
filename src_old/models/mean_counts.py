"""
An MCMC implementation of the negative binomial mean 
counts model for fitting to the NFPD counts data.

"""

import numpy as np
import pandas as pd
import scipy.special as spec


def loglikeNegBinom(
    n: np.ndarray,
    k: np.ndarray,
    m: np.ndarray,
) -> np.ndarray:
    """
    A custom vectorised negative binomial log-likelihood function
    to avoid any scipy definition ambiguities.

    Args:
    n
        the count variable numpy array
    k
        the aggregation parameter numpy array
    m
        the mean parameter numpy array

    """

    # Set mean and variance
    mean, var = m, m + (m ** 2.0 / k)

    # Negative binomial loglikelihood with mean and variance specified
    sol = np.log(
        (
            spec.gamma(((mean ** 2.0) / (var - mean)) + n)
            / (spec.gamma(n + 1.0) * spec.gamma(((mean ** 2.0) / (var - mean))))
        )
        * ((mean / var) ** ((((mean ** 2.0) / (var - mean)))))
        * (((var - mean) / var) ** n)
    )

    # If any overflow problems, use large argument expansion
    overflow_vals = np.isnan(sol) | np.isinf(sol)
    overflow_n = n[overflow_vals]
    sol[overflow_vals] = np.log(
        (
            ((1.0 - (mean[overflow_vals] / var[overflow_vals])) ** overflow_n)
            * (
                overflow_n
                ** (
                    (
                        mean[overflow_vals] ** 2.0
                        / (var[overflow_vals] - mean[overflow_vals])
                    )
                    - 1.0
                )
            )
            * (
                (mean[overflow_vals] / var[overflow_vals])
                ** (
                    mean[overflow_vals] ** 2.0
                    / (var[overflow_vals] - mean[overflow_vals])
                )
            )
            / (
                spec.gamma(
                    mean[overflow_vals] ** 2.0
                    / (var[overflow_vals] - mean[overflow_vals])
                )
            )
        )
    )

    # Avoiding further pathologies
    sol[(var < mean)] = -np.inf
    sol[np.isnan(sol)] = -np.inf

    return sol
