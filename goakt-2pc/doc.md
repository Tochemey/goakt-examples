# goakt-2pc: 2 Phase Commit Money Transfer

A production-like example implementing a money transfer application using the **2 Phase Commit (2PC) pattern** with GoAkt actors. The
example runs on Kubernetes using Kind and uses only Go types for actor messages (no protobuf).

For a detailed explanation of the application and how the 2PC pattern is used, see **[2PC.md](./2PC.md)**.

## 2 Phase Commit Pattern (Summary)

The 2PC pattern coordinates distributed transactions across multiple services/actors with atomic consistency. For a money transfer:

1. **Phase 1 (Prepare/Vote)**: The coordinator asks both accounts to prepare the transfer
   - Source account validates it can debit the amount, votes YES or NO
   - Destination account validates it can receive the credit, votes YES or NO
   - If any participant votes NO, the transfer is aborted

2. **Phase 2 (Commit/Abort)**: If all participants voted YES
   - Send commit to both accounts to apply the changes
   - If any participant voted NO or failed
   - Send abort to all participants to release locks without applying changes

The `Coordinator` actor manages this two-phase protocol and persists transfer state to PostgreSQL.

## Architecture

- **AccountEntity**: Manages a single account (create, prepare, commit, abort, get). State persisted to PostgreSQL.
- **Coordinator**: One coordinator per transfer (actor name = transfer ID). Manages the 2PC protocol:
  - Phase 1: Sends prepare requests, collects votes
  - Phase 2: Sends commit if all voted YES, abort if any voted NO
- **Persistence**: PostgreSQL stores accounts and transfer state.
- **Cluster**: Kubernetes discovery, 3 replicas, HTTP/JSON API.

## API Endpoints

| Method | Path                  | Description                     |
|--------|-----------------------|---------------------------------|
| POST   | /accounts             | Create account                  |
| GET    | /accounts/{id}        | Get account balance             |
| POST   | /accounts/{id}/credit | Direct credit (bypass 2PC)      |
| POST   | /transfers            | Initiate two-pc-based transfer  |
| GET    | /transfers/{id}       | Get transfer status             |

## Running on Kind

```bash
# From goakt-2pc/
make cluster-create  # One-time: create Kind cluster
make deploy          # Build image, load, deploy
make port-forward    # Access API at http://localhost:8080
make test            # Run integration tests
make cluster-down    # Tear down
```

## Prerequisites

- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Earthly](https://earthly.dev/get-earthly) (for building the Docker image)

## Message Types (Go structs)

All actor messages are Go types, registered for remoting serialization:

### Account Messages
- `CreateAccount`, `GetAccount`, `Account`

### 2PC Transfer Messages
- `StartTransfer` - Initiates a new transfer
- `PrepareTransfer` - Phase 1: Ask participant to prepare and vote
- `CommitTransfer` - Phase 2: Tell participant to commit changes
- `AbortTransfer` - Phase 2: Tell participant to abort and release locks
- `VoteYes` - Participant response: can commit
- `VoteNo` - Participant response: cannot commit
- `TransferCompleted` - Transfer successfully committed
- `TransferFailed` - Transfer aborted
- `GetTransferStatus`, `TransferStatus` - Query transfer state

## Limitations

This project is a Proof of Concept for building 2 Phase Commit system using Goakt. This project has the following (but not limited to) limitations:

- Safely retry the transfer when the actor/host dies and respawn on a different node

I recommends you solve the potential problems above before adopting this project/concept in your production system.
