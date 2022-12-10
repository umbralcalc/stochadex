import numpy as np
import scipy.special as sps
from utils import plot_x_time_series


def main():
    plot_x_time_series(
        10.0,
        0.1,
        np.zeros((2, 3)),
        np.zeros((2, 3)),
        lambda X_prime, t, delta_t: (
            X_prime[0]
            - delta_t
            * np.asarray([1.0, 2.0, 3.0])
            * (X_prime[0] - np.asarray([3.0, 5.0, 2.0]))
        ),
        lambda X_prime, S_prime, t, delta_t, other: (
            np.sqrt(delta_t)
            * np.random.normal(
                np.zeros(3),
                np.asarray([0.5, 1.0, 0.5]),
                size=3,
            )
        ),
        "docs/images/wiener_noise.png",
        [
            "Ornstein-Uhlenbeck (1.0, 3.0, 0.5)",
            "Ornstein-Uhlenbeck (2.0, 5.0, 1.0)",
            "Ornstein-Uhlenbeck (3.0, 2.0, 0.5)",
        ],
    )
    plot_x_time_series(
        10.0,
        0.1,
        np.zeros((2, 3)),
        np.zeros((2, 3)),
        lambda X_prime, t, delta_t: (
            X_prime[0]
            - delta_t
            * np.asarray([1.0, 2.0, 3.0])
            * (X_prime[0] - np.asarray([3.0, 5.0, 2.0]))
        ),
        lambda X_prime, S_prime, t, delta_t, other: (
            np.sqrt(delta_t)
            * X_prime[0]
            * np.random.normal(
                np.zeros(3),
                np.asarray([0.5, 1.0, 0.5]),
                size=3,
            )
        ),
        "docs/images/gbm_noise.png",
        [
            "GBM with drift (1.0, 3.0, 0.5)",
            "GBM with drift (2.0, 5.0, 1.0)",
            "GBM with drift (3.0, 2.0, 0.5)",
        ],
    )
    # note method 2 of simulation here: https://en.wikipedia.org/wiki/Fractional_Brownian_motion
    t_period = 10.0
    t_step = 0.1

    def K(H: float, t: np.ndarray, s: np.ndarray) -> np.ndarray:
        output = (
            ((t - s) ** (H - 0.5))
            * sps.hyp2f1(H - 0.5, 0.5 - H, H + 0.5, 1.0 - t / s)
            / sps.gamma(H + 0.5)
        )
        output[s == 0] = 1.0
        return output

    def brownian_motion_iterate(previous: np.ndarray) -> np.ndarray:
        new = np.roll(previous, 1, axis=0)
        new[0] = np.sqrt(t_step) * np.random.normal(
            np.zeros(3),
            np.asarray([0.5, 1.0, 0.5]),
            size=3,
        )
        return new

    def fractional_brownian_motion(
        X_prime: np.ndarray,
        S_prime: np.ndarray,
        t: float,
        delta_t: float,
        other: np.ndarray,
    ) -> np.ndarray:
        fBm_vector = -S_prime[0]
        n = int(np.round(t / delta_t, 1))
        H_values = [0.8, 0.7, 0.3]
        for i in range(3):
            if t == 0:
                fBm_vector[i] = other[0, i]
                continue
            fBm_vector[i] += np.sum(
                K(H_values[i], t, delta_t * np.arange(0, n)) * other[:n, i]
            )  # 'other' is the Brownian motion current windowed history
        return fBm_vector

    plot_x_time_series(
        t_period,
        t_step,
        np.zeros((2, 3)),
        np.zeros((2, 3)),
        lambda X_prime, t, delta_t: (
            X_prime[0]
            - delta_t
            * np.asarray([1.0, 2.0, 3.0])
            * (X_prime[0] - np.asarray([3.0, 5.0, 2.0]))
        ),
        fractional_brownian_motion,
        "docs/images/fbm_noise.png",
        [
            "fBM with drift (1.0, 3.0, 0.5, H=0.8)",
            "fBM with drift (2.0, 5.0, 1.0, H=0.7)",
            "fBM with drift (3.0, 2.0, 0.5, H=0.3)",
        ],
        other_noise_init=np.zeros((int(t_period / t_step) + 1, 3)),
        other_noise_iterate=brownian_motion_iterate,
    )


if __name__ == "__main__":
    main()
