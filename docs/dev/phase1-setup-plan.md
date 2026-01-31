# Phase 1 Setup Plan: Project Structure Setup

## Task 1.1: Copy Base Files

### Objective
Create the curp-ht package by copying the existing CURP implementation files as a base.

### Files to Copy

| Source | Destination |
|--------|-------------|
| curp/curp.go | curp-ht/curp-ht.go |
| curp/client.go | curp-ht/client.go |
| curp/defs.go | curp-ht/defs.go |
| curp/batcher.go | curp-ht/batcher.go |
| curp/timer.go | curp-ht/timer.go |

### Steps
1. Ensure curp-ht directory exists
2. Copy each file with appropriate renaming
3. Verify all files are copied correctly

### Verification
- All 5 files exist in curp-ht/
- Files have correct content (same as source minus package/import changes in next task)

---

## Task 1.2: Update Package Names and Imports

### Objective
Update all copied files to use the new package name `curpht` and correct import paths.

### Changes Required

#### Package Declaration
Change in all files:
```go
// From:
package curp

// To:
package curpht
```

#### Import Path Updates
No import path changes needed for internal imports since curp-ht files don't import each other using full paths (they use relative package references).

The external imports (github.com/imdea-software/swiftpaxos/...) remain unchanged.

### Files to Modify
1. curp-ht/curp-ht.go - package declaration
2. curp-ht/client.go - package declaration
3. curp-ht/defs.go - package declaration
4. curp-ht/batcher.go - package declaration
5. curp-ht/timer.go - package declaration

### Verification
- `go build ./curp-ht` succeeds (after all tasks in Phase 1)
- Package is importable as `github.com/imdea-software/swiftpaxos/curp-ht`
