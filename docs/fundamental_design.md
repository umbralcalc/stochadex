# The fundamental design

The stochadex has been designed to try and maintain a balance between performance and flexibility of utilisation (it's meant to be a general stochastic modelling tool after all!). If we jump straight into the approach, let's write very general formula for iterating an element from some matrix $X$ forward one finite step in time

$$
\begin{equation}
X_{i(t+1)} = X_{it} + {\cal F}_{it}(X) + {\cal G}_{it}(X) \big[ D_{i(t+1)}-D_{it}\big] + {\cal H}_{it}(X)\big[ J_{i(t+1)}-J_{it}\big] \,,
\end{equation}
$$

where at this stage we can define

- $i$ as an index for an arbitrarily-sized state space
- $t$ as an index for the current time step number
- ${\cal F}_{it}(X)$ as some matrix-valued function whose output carries dimensions of $[{\rm time}]$ and whose input argument is the entire matrix $X$
- ${\cal G}_{it}(X)$ and ${\cal H}_{it}(X)$ as some matrix-valued functions, each of which with dimensionless outputs and an input argument that is the entire matrix $X$
- $D_{it}$ as the recorded values of some arbitrary continuous-time stochastic process (e.g., Wiener process)
- and $J_{it}$ as the recorded values of some arbitrary non-continuous-time stochastic process (e.g., Poisson process)
