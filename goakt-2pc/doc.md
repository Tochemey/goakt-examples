# goakt-2pc: Saga-based Money Transfer

A production-like example implementing a money transfer application using the **saga pattern** with GoAkt actors. The
example runs on Kubernetes using Kind and uses only Go types for actor messages (no protobuf).

For a detailed explanation of the application and how the saga pattern is used, see **[SAGA.md](./SAGA.md)**.

## Saga Pattern (Summary)

The saga pattern coordinates distributed transactions across multiple services/actors. For a money transfer:

1. **Step 1**: Debit the source account
2. **Step 2**: Credit the destination account
3. **Compensation**: If step 2 fails, credit back the source account (undo step 1)

Each step is executed via `Ask` to the corresponding `AccountEntity` actor. The `SagaOrchestrator` actor coordinates the
flow and persists transfer state to PostgreSQL.

## Architecture

- **AccountEntity**: Manages a single account (create, debit, credit, get). State persisted to PostgreSQL.
- **SagaOrchestrator**: One orchestrator per transfer (actor name = transfer ID). Executes the saga steps and
  compensates on failure.
- **Persistence**: PostgreSQL stores accounts and transfer state.
- **Cluster**: Kubernetes discovery, 3 replicas, HTTP/JSON API.

## API Endpoints

| Method | Path                  | Description                  |
|--------|-----------------------|------------------------------|
| POST   | /accounts             | Create account               |
| GET    | /accounts/{id}        | Get account balance          |
| POST   | /accounts/{id}/credit | Direct credit (bypass saga)  |
| POST   | /transfers            | Initiate saga-based transfer |
| GET    | /transfers/{id}       | Get transfer status          |

## Running on Kind

```bash
# From goakt-2pc/
make cluster-create   # One-time: create Kind cluster
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

- `CreateAccount`, `DebitAccount`, `CreditAccount`, `GetAccount`, `Account`
- `StartTransfer`, `TransferCompleted`, `TransferFailed`
- `GetTransferStatus`, `TransferStatus`
