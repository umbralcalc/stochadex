# Configurations of the stochadex

Let's make some noise.
## Flavours of noise with continuous sample paths

### Wiener process noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{W^i_{t+1}}-\textcolor{red}{W^i_{t}},
\end{align}
$$

where $W^i_{t}$ is a sample from a Wiener process for each of the state dimensions indexed by $i$. Note that we may also allow for correlations between the noises in different dimensions.

### Geometric Brownian motion noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{X^i_{t}W^i_{t+1}}-\textcolor{red}{X^i_{i-1}W^i_{t}}.
\end{align}
$$


### Fractional Brownian motion noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{B^i_{t+1}({H_i})}-\textcolor{red}{B^i_{t}({H_i})},
\end{align}
$$

where $B^i_{t}({H_i})$ is a sample from a fractional Brownian motion process with Hurst exponent $H_i$ for each of the state dimensions indexed by $i$.

### Generalised continuous noises

In the stochadex formalism, these generally could take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{g^i_{t+1}(X', W^i_{t+1}, \dots)}-\textcolor{red}{g^i_{t}(X'', W^i_{t}, \dots)},
\end{align}
$$

where $g^i_{t+1}(X', W^i_{t+1}, \dots)$ is some continuous function of its arguments which can be expanded out with [Itôs Lemma](https://en.wikipedia.org/wiki/It%C3%B4%27s_lemma).

### Jump process noises

In the stochadex formalism, these generally could take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{J^i_{t+1}(X', \dots )}-\textcolor{red}{J^i_{t}(X'', \dots )},
\end{align}
$$

where $J^i_{t+1}(X', \dots )$ are samples from some arbitrary jump process (e.g., compound Poisson) which could generally depend on a variety of inputs, including $X'$. 


## Flavours of noise with discontinuous sample paths

### Poisson process noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{N^i_{t+1}({\lambda_i})}-\textcolor{red}{N^i_{t}({\lambda_i})},
\end{align}
$$

where $N^i_{t}({\lambda_i})$ is a sample from a Poisson process with rate $\lambda_i$ for each of the state dimensions indexed by $i$. Note that we may also allow for correlations between the noises in different dimensions.

### Time-inhomogeneous Poisson process noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{N^i_{t+1}({\lambda^i_{t+1}})}-\textcolor{red}{N^i_{t}({\lambda^i_t})},
\end{align}
$$

where $\lambda^i_{t}$ is a deterministically-varying rate for each of the state dimensions indexed by $i$.

### Cox (doubly-stochastic) process noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{N^i_{t+1}({\Lambda^i_{t+1}})}-\textcolor{red}{N^i_{t}({\Lambda^i_{t}})},
\end{align}
$$

where the rate $\Lambda^i_{t}$ is now a sample from some continuous-time stochastic process (in the positive-only domain) for each of the state dimensions indexed by $i$.

### Self-exciting process noise

In the stochadex formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + \textcolor{red}{N^i_{t+1}[{\cal I}^i_{t+1}(N', \dots)]}-\textcolor{red}{N^i_{t}[{\cal I}^i_{t}(N'', \dots)]},
\end{align}
$$

where the stochastic rate ${\cal I}^i_{t}(N', \dots)$ now depends on the history of $N'$ explicitly (amongst other potential inputs - see, e.g., [Hawkes processes](https://en.wikipedia.org/wiki/Hawkes_process)) for each of the state dimensions indexed by $i$.

### Generalised probabilistic discrete state transitions

In the stochadex formalism, these generally can take the form

$$
\begin{align}
& X^i_{t+1} = X^i_{t} + \cancel{F^i_{t+1}(X')} + \textcolor{red}{T^i_{t+1}(X')}-\textcolor{red}{X^i_{t}},
\end{align}
$$

where $T^i_{t+1}(X')$ is a generator of the next state to occupy. This generator uses the current state transition probabilities (which are generally conditional on $X'$) at each new step.