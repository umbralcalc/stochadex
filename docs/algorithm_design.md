# Core algorithm design

## General mathematical formalism

The stochadex has been designed to try and maintain a balance between performance and flexibility of utilisation (it's meant to be a general stochastic modelling tool after all!). If we jump straight into the approach, let's write very general formula for iterating forward one finite step in time and adding a new row to some matrix $X$

$$
\begin{align}
& S''\longrightarrow S' \longrightarrow S \\
& X''\longrightarrow X' \longrightarrow X \\
& X^i_{t+1} = (X')^i_{t} + F^i_{t+1}(X') + S^i_{t+1}(X',S')-S^i_{t}(X'',S'') \,,
\end{align}
$$

where at this stage we can define

- $i$ as an index for an arbitrarily-sized state space
- $t$ as an index for the current time step number
- $F^i_{t+1}(X')$ as the latest element of some matrix which carries dimensions of $[{\rm time}]$ and may generally depend on $X'$
- $S^i_{t+1}(X', S')$ as the values of a stochastic process over the state space indexed by $i$ which may either vary continuously in time (giving it dimensions of $[{\rm time}^{1/2}]$) or not (giving it dimensions of $[{\rm time}]$), and which generally depend on $X'$ and $S'$ 


## Design of a timestep

Describe the process

Insert diagram here



