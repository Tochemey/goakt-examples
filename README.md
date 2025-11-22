# About

[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/goakt-examples)](https://go.dev/doc/install)

This repo contains examples for [Go-Akt](https://github.com/Tochemey/goakt). All the examples here target Go-Akt latest release

## Installation
To download the examples code:

```bash
 cd $HOME/examples
 git clone https://github.com/Tochemey/goakt-examples
```

### Building

Before building and running the examples you need to install [Earthly](https://earthly.dev/get-earthly).

Run the following command:
`earthly +all`

## Examples
Click links below for more details on how to run each example.

1. [Hello World](./goakt-actor-hello-world)
2. [Actors Clustering](./goakt-actors-cluster)
   - [Kubernetes Discovery](./goakt-actors-cluster/k8s)
   - [Static Discovery](./goakt-actors-cluster/static)
   - [DNS Discovery](./goakt-actors-cluster/dnssd)
   - [Location Transparent](./goakt-actors-cluster/dynalloc)
3. [Remoting](./goakt-remoting)
4. [Messaging](./goakt-ping-pong)
5. [Behavior](./goakt-actor-behaviors)
6. [Chat](./goakt-chat)
7. [Persistence using Extension](./goakt-actor-persistence)
8. [Grains](./goakt-grains)
9. [Grains Clustering](goakt-grains-cluster/grains-dnssd)