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

1. [Hello World](./actor-hello-world)
2. [Clustering](./actor-cluster)
   - [Kubernetes Discovery](./actor-cluster/k8s)
   - [Static Discovery](./actor-cluster/static)
   - [DNS Discovery](./actor-cluster/dnssd)
3. [Remoting](./actor-remoting)
4. [Messaging](./actor-to-actor)
5. [Simple Receive](./actor-receive)
6. [Behavior](./actor-behaviors)
7. [Chat](./actor-chat)
8. [Persistence using Extension](./actor-persistence)
9. [Grains](./grains)
10. [Cluster Grains](./actor-cluster/grains-dnssd)