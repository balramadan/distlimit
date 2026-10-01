# Contributing to `distlimit`

Thank you for your interest in contributing to **`distlimit`**! We welcome contributions from the community to help make this rate limiting library faster, more resilient, and easier to use.

Please take a few moments to review these guidelines before submitting an issue or pull request.

---

## 🧭 Core Principles & Architecture Philosophy

Every contribution to `distlimit` must adhere to these foundational engineering standards:

1. **⚡ Nanosecond Latency & Zero Memory Overhead:**
   - Hot-path operations (algorithm evaluations, key extraction, metrics observation) must strive for **`0 B/op`** and **`0 allocs/op`** wherever possible.
   - Avoid unnecessary heap allocations, reflection, or unbuffered string conversions inside the request lifecycle.
2. **🔒 Concurrency & Thread-Safety:**
   - All state modifications must be safe for concurrent access.
   - Favor atomic operations (e.g., `atomic.Pointer`, `atomic.Int64`) and fine-grained sharding over global locks to prevent lock contention.
   - Never introduce code that fails `go test -race`.
3. **🔐 Security First:**
   - Network address extraction, reverse proxy headers (`X-Forwarded-For`, `X-Real-IP`), and CIDR filtering must strictly prevent spoofing or injection attacks.
4. **🔌 Modularity & Backward Compatibility:**
   - Public APIs (`distlimit.Limiter`, middleware options, storage driver contracts) should remain backward-compatible across minor and patch versions.

---

## 🛠️ Getting Started with Local Development

### Prerequisites

- **Go:** Version `1.23+` (recommended Go `1.24+` or `1.26+`).
- **Redis:** Version `6.2+` or `7.x+` (optional, for Redis driver integration tests).
- **Git:** Standard git version control.

### Cloning & Setup

```bash
# 1. Clone the repository or your fork
git clone https://github.com/balramadan/distlimit.git
cd distlimit

# 2. Download dependencies
go mod download
go mod verify
```

### Running Tests & Verification

Before submitting any code, always run the full test suite with the race detector and Go's static analysis tools:

```bash
# Run all unit tests with race detection
go test -v -race ./...

# Run static analysis
go vet ./...

# Run benchmarks to check memory allocations
go test -bench=. -benchmem ./...
```

---

## 📋 Contribution Workflow

### 1. Reporting Bugs & Vulnerabilities

- **Bug Reports:** Open an issue on GitHub detailing your environment (Go version, OS, framework adapter), a minimal reproducible example, and expected vs actual behavior.
- **Security Vulnerabilities:** If you discover a security flaw (such as an IP spoofing bypass, denial-of-service vector, or authentication issue), please report it responsibly by contacting the maintainer via GitHub ([@balramadan](https://github.com/balramadan)) or via private vulnerability reporting before posting publicly.

### 2. Proposing Features

For major feature proposals (e.g., a new storage driver, new rate limiting algorithm, or architectural refactor), please open an **Issue / RFC** first to discuss the design and trade-offs before writing extensive code.

### 3. Submitting Pull Requests

1. **Create a topic branch:**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```
2. **Write clean, idiomatic Go:**
   - Format all Go code with standard formatting tools (`gofmt -s -w .` or `goimports`).
   - Add clear docstrings and comments adhering to standard Go conventions.
   - Preserve existing documentation integrity and comments.
3. **Add Tests:**
   - Every bug fix must include a regression test proving the fix works.
   - Every new feature must include comprehensive unit tests and edge-case coverage.
4. **Verify Benchmarks:**
   - If touching core rate limiting logic, ensure benchmarks do not show memory allocation regressions.
5. **Open the PR:**
   - Provide a clear, detailed PR description describing *what* changed, *why*, and *how* it was tested.

---

## 🧪 Code Style & Guidelines

- **Imports Grouping:** Group imports into standard library, third-party libraries, and local packages.
- **Error Handling:** Return descriptive errors. Use sentinel errors or typed errors when callers need to distinguish error cases.
- **Testing:** Prefer table-driven tests (`tests := []struct{...}`) for multiple test cases.

---

## 📜 Code of Conduct

Participation in this project is governed by the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By contributing, you agree to uphold its values.
