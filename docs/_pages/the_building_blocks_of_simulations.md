---
title: "The building blocks of simulations"
logo: true
---

# The building blocks of simulations
<div style="height:0.75em;"></div>

## Time and its 'history'

Time is probably the most important variable to track in a simulation. This is because everything that happens within a simulation only has meaning with respect to the real world if we can trace what point in simulated Time that it happened.

Time is a variable in the simulation which always increases in value. We often want to track how it increases after each discrete simulation 'step' in increments which we shall call 'Timesteps'.

The first data structure which we shall introduce here is a 'Cumulative Timesteps History', which indexes simulated Times that the simulation _had_ according to _decreasing recency_.

<img src="../assets/cumulative-timesteps-history.svg" />

## State partitions and their history

Simulations also have a 'State' associated to them for each value of Time.

A simulation State holds all of the essential information we want to track to make the simulation useful to us which, crucially, includes _all of the information that is needed for the simulation to proceed_.

In order to simplify how all of this State data is processed and retrieved, we're going to _partition_ this simulation State into several 'State Partition' data structures.

For example, one of these partitions might hold all the data associated to a player in a sports team, and the whole simulation needs all of the players within its overall State.

<img src="../assets/state-partitions.svg" />

Each State Partition carries data that can be different for each moment in Time of the simulation. In order to track this evolution, we create a 'State Partition History' data structure for each State Partition.

Each State Partition History has indices which always match those of the Cumulative Timesteps History, so they are always synchronised with each other.

<img src="../assets/state-partition-history.svg" />

## Computing the next timesteps

So new States of the simulation may happen at different Times. But what determines when these Times are along the Simulation Timeline?

When simulating systems in the real world, it can be desireable in some situations to be able to structure the Times along the Timeline at regular intervals.

In other situations it can be desirable to have an unstructured or randomly-drawn set of Times along the Timeline.

<img src="../assets/timeline.svg" />

In order to allow this flexibility in choice between situations, let's specify an 'Advance Time' calculation which takes in the current Cumulative Timesteps History and outputs the Next Timestep following the latest Time found in the History (at index 0).

<img src="../assets/advance-time.svg" />

## State partition iteration

For an overall simulation data view, we can create a 'State Partition Histories' structure. This is a collection of each State Partition History that exists within the simulation.

<img src="../assets/state-partition-histories.svg" />

How do we determine the 'Next Values' of each State Partition in Time?

This is where we should maintain full flexibility in choice of calculation so that our simulation is able to represent many different real-world systems.

Each of these calculations for a given Partition specifies how the State Partition Histories and Cumulative Timesteps History data are used to determine the Next Values of the State Partition.

In order to support even more flexibility in configuration, the calculation should also take in a set of Parameters which are specific to that Partition.

<img src="../assets/partition-iteration.svg" />

## State partition and time updates

Having calculated the Next Values of each State Partition and the Next Timestep, we should perform operations over the Cumulative Timesteps History and each State Partition History to Update their structures with this new data.

This is an important procedure to perform all at once to ensure that the all of the State Partitions in the simulation are synchronised against its virtual clock.

<img src="../assets/update-time-history.svg" />

<img src="../assets/update-state-partition-history.svg" />

## Composing and coordinating partitions

So far we have considered Partition Iterations as occurring seperately within the simulation. But it is important for these separate Partitions to be able to pass information to each other. 

To understand why this is important, think of our sports team example earlier and how each player might change their behaviour depending on the positions of other players on the field. They need to know about each other.

So how do we compose Partitions together so that information may be passed between them? 

In some sense, this happens already. Each Partition may read information from the State Partition Histories in the Iteration to its Next Values.

<img src="../assets/partition-timeline-composition.svg" />

This composition allows us to correlate the behaviour of Partitions in Time.

We might also want to coordinate the Iterations of Partitions to occur in a particular order, where the first Iteration may then pass its calculated Next Values immediately into the second.

<img src="../assets/partition-dependencies.svg" />

For example, an Iteration might calculate the Mean statistic from some data and another might calculate the Variance. The Variance depends on the Mean so we need the Mean Next Values to be passed directly into the Variance to make sure these are always aligned in Time.

To enable coordination, all we need is for a 'Computationally Upstream' Iteration to be allowed to pass its Next Values into the Parameters of a 'Computationally Downstream' Iteration.

<img src="../assets/composing-partitions.svg" />

## Simulation runs

A 'Simulation Run' defines the evolution of a simulation State over some specified period in Time. 

Each Simulation Run has an Initial Time defined by an exact value, but the _end_ Time may not be known exactly until the simulation has evolved to meet some Termination Condition.

<img src="../assets/simulation-run.svg"/>

The Termination Condition logically determines when the Simulation Run should end according to a pre-defined calculation.

<img src="../assets/termination-condition.svg"/>

## Embedded simulation runs

<img src="../assets/embedded-simulation-run.svg"/>

<img src="../assets/embedded-sim-iterate.svg" width=600/>

<img src="../assets/embedded-sim-from-history.svg" width=600/>
