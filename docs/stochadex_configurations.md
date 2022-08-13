# Configurations of the stochadex

Let's make some noise.
## Flavours of continuous noise

### Wiener process noise

In the algorithm formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + \textcolor{red}{W^i_{t+1}}-\textcolor{red}{(W')^i_{t}},
\end{align}
$$

where $W^i_{t+1}$ is a sample from a Wiener process for each of the states indexed by $i$. Note that we may also allow for correlations between the noises in different dimensions.

### Geometric Brownian motion noise

In the algorithm formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + \textcolor{red}{(X')^i_{t}W^i_{t+1}}-\textcolor{red}{(X'')^i_{i-1}(W')^i_{t}}.
\end{align}
$$


### Fractional Brownian motion noise

In the algorithm formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + \textcolor{red}{(B_{H_i})^i_{t+1}}-\textcolor{red}{(B'_{H_i})^i_{t}},
\end{align}
$$

where $(B_{H_i})^i_{t+1}$ is a sample from a fractional Brownian motion process with Hurst exponent $H_i$ for each of the states indexed by $i$. Note that we may also allow for correlations between the noises in different dimensions.

### Generalised continuous noises

In the algorithm formalism, these generally could take the form

$$
\begin{align}
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + \textcolor{red}{g^i_{t+1}(X', W^i_{t+1})}-\textcolor{red}{g^i_{t}(X'', W^i_{t})},
\end{align}
$$

where $g^i_{t+1}(X', W^i_{t+1})$ is some continuous function of $X'$ and $W^i_{t+1}$.

## Flavours of jump process noise

### Poisson process noise

In the algorithm formalism, these generally take the form

$$
\begin{align}
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + \textcolor{red}{N^i_{t+1}}-\textcolor{red}{(N')^i_{t}},
\end{align}
$$

where $N^i_{t+1}$ is a sample from a Poisson process for each of the states indexed by $i$. Note that we may also allow for correlations between the noises in different dimensions.