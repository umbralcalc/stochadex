# Core algorithm design

## General mathematical formalism

The stochadex has been designed to try and maintain a balance between performance and flexibility of utilisation (it's meant to be a general stochastic modelling tool after all!). If we jump straight into the approach, let's write very general formula for iterating forward one finite step in time and adding a new row to some matrix $X$

$$
\begin{align}
& S''\longrightarrow S' \longrightarrow S \\
& X''\longrightarrow X' \longrightarrow X \\
& X^i_{t+1} = X^i_{t} + F^i_{t+1}(X') + S^i_{t+1}(X',S')-S^i_{t}(X'',S''),
\end{align}
$$

where at this stage we can define

- $i$ as an index for the dimensions of the state space
- $t$ as an index for the current time step number which may either correspond to a discrete-time process or discrete approximation to a continuous-time process
- $F^i_{t+1}(X')$ as the latest element of some matrix which very generally can depend on $X'$
- $S^i_{t+1}(X', S')$ as the recorded values of a stochastic process defined over the dimensions of the state space indexed by $i$ which may either vary with continuous sample paths in time or not, and which very generally can depend on $X'$ and $S'$

## Design of a timestep

Describe the process

Insert diagram here



