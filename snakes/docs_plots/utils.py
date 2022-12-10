from typing import Tuple, List, Optional
import matplotlib.pyplot as plt
import numpy as np


def get_next_matrix_row(
    t: float,
    delta_t: float,
    X_prime: np.ndarray,
    S_prime: np.ndarray,
    F: callable,
    S: callable,
    other_noise_prime: Optional[np.ndarray] = None,
    other_noise_iterate: Optional[callable] = None,
) -> Tuple[np.ndarray, np.ndarray]:
    if other_noise_prime is not None:
        other = other_noise_iterate(other_noise_prime)
    else:
        other = None
    s = S(X_prime, S_prime, t, delta_t, other)
    x = F(X_prime, t, delta_t) + s
    return x, s


def generate_x_time_series(
    t_period: float,
    delta_t: float,
    X_init: np.ndarray,
    S_init: np.ndarray,
    F: callable,
    S: callable,
    other_noise_init: Optional[np.ndarray] = None,
    other_noise_iterate: Optional[callable] = None,
) -> List[np.ndarray]:
    x_time_series = []
    X_current, S_current, other_noise_current = X_init, S_init, other_noise_init
    t = 0.0
    while t < t_period:
        x, s = get_next_matrix_row(
            t=t,
            delta_t=delta_t,
            X_prime=X_current,
            S_prime=S_current,
            F=F,
            S=S,
            other_noise_prime=other_noise_current,
            other_noise_iterate=other_noise_iterate,
        )
        x_time_series.append(x)
        X_current = np.roll(X_current, 1, axis=0)
        S_current = np.roll(S_current, 1, axis=0)
        X_current[0] = x
        S_current[0] = s
        t += delta_t
    return x_time_series


def plot_x_time_series(
    t_period: float,
    delta_t: float,
    X_init: np.ndarray,
    S_init: np.ndarray,
    F: callable,
    S: callable,
    filename: str,
    names: List[str],
    other_noise_init: Optional[np.ndarray] = None,
    other_noise_iterate: Optional[callable] = None,
):
    x_time_series = generate_x_time_series(
        t_period=t_period,
        delta_t=delta_t,
        X_init=X_init,
        S_init=S_init,
        F=F,
        S=S,
        other_noise_init=other_noise_init,
        other_noise_iterate=other_noise_iterate,
    )
    fig, ax = plt.subplots(1, 1, figsize=(10, 5))
    for i in range(len(x_time_series[0])):
        ax.plot(
            np.arange(0, t_period + delta_t, delta_t),
            np.asarray(x_time_series).T[i],
            label=names[i],
        )
    plt.savefig(filename)
