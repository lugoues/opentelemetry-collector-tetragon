# Coding Conventions

**Analysis Date:** 2026-03-18

## Status

This is a GSD framework setup repository with no actual project source code. Analysis is based on the minimal JavaScript hook files present in `.claude/hooks/`.

## Naming Patterns

**Files:**
- kebab-case for Node.js executable files (e.g., `gsd-check-update.js`, `gsd-context-monitor.js`, `gsd-statusline.js`)
- Prefix with `gsd-` for GSD framework integration hooks

**Functions:**
- camelCase for function declarations
- Example: `detectConfigDir()`, `clearTimeout()`, `JSON.parse()`

**Variables:**
- camelCase for local variables and constants
- Examples: `homeDir`, `cwd`, `cacheDir`, `cacheFile`, `projectVersionFile`
- UPPERCASE_SNAKE_CASE for constant values that represent thresholds or configuration
  - Example: `WARNING_THRESHOLD`, `CRITICAL_THRESHOLD`, `AUTO_COMPACT_BUFFER_PCT`

**Types:**
- Plain JavaScript objects used instead of TypeScript
- No explicit type annotations

## Code Style

**Formatting:**
- No linter config detected (`.eslintrc*`, `.prettierrc` not present)
- Manual formatting observed in hook files
- 2-space indentation
- Standard Node.js style conventions

**Linting:**
- Not detected in repository

**Semicolons:**
- Used consistently throughout hook files
- Example: `const fs = require('fs');`

## Import Organization

**Order:**
1. Node.js built-in modules (e.g., `fs`, `path`, `os`, `child_process`)
2. Local module logic follows imports

**Pattern:**
```javascript
const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawn } = require('child_process');
```

**Path Aliases:**
- Not used; all requires are relative or built-in modules

## Error Handling

**Patterns:**
- Try-catch blocks used for file system operations and process invocations
- Silent failures in utility code with comment explanations
  - Example in `gsd-statusline.js`: "Silent fail -- bridge is best-effort, don't break statusline"
- Process exit on critical failures (e.g., timeout scenarios)
- Example: `setTimeout(() => process.exit(0), 3000)` for stdin timeout guard

**Error Prevention:**
- File existence checks before reading/writing
  - Pattern: `if (!fs.existsSync(cacheDir)) { fs.mkdirSync(cacheDir, { recursive: true }); }`
- Timeout guards for I/O operations to prevent hanging
  - Pattern: `{ timeout: 10000 }` in `execSync` calls

## Logging

**Framework:** console and process.stdout

**Patterns:**
- Direct output via `process.stdout.write()` for formatted results
- No dedicated logging framework
- Color codes in terminal output using ANSI escape sequences
  - Example: `\x1b[32m` (green), `\x1b[33m` (yellow), `\x1b[31m` (red)

**Guidelines:**
- Output JSON to files instead of console for inter-process communication
- Silent failures preferred in utility hooks to avoid breaking calling processes

## Comments

**When to Comment:**
- Document non-obvious logic, especially around system interaction
- Explain design decisions (e.g., why silent failures are appropriate)
- Note platform-specific behavior
  - Example: "// Required on Windows for proper process detachment"

**Style:**
- Single-line comments with `//` for inline explanation
- Multi-line comment blocks for complex logic sections
- Example from `gsd-check-update.js`:
  ```javascript
  // Check project directory first (local install), then global
  let installed = '0.0.0';
  ```

**JSDoc/TSDoc:**
- Not used; project is plain JavaScript without type annotations

## Function Design

**Size:**
- Small, focused functions (50-100 lines typical)
- Each function has single responsibility

**Parameters:**
- Minimal parameters, preferring to read from environment or files
- Example: `detectConfigDir(baseDir)` takes single parameter

**Return Values:**
- Functions return data objects (JSON-serializable)
- Example return: `{ update_available: boolean, installed: string, latest: string, checked: number }`

## Module Design

**Exports:**
- Executables use shebang and direct execution (`#!/usr/bin/env node`)
- No explicit module.exports; files are run as scripts
- All files in `.claude/hooks/` are self-contained entry points

**Barrel Files:**
- Not applicable; hook files are not modularized

## Special Patterns

**Detached Process Spawning:**
- Used in background operations
- Pattern: `spawn(...).unref()` to detach from parent process

**Environment Variable Handling:**
- Flexible config directory detection via `CLAUDE_CONFIG_DIR` env var
- Fallback chain for multiple possible locations
- Example from `detectConfigDir()`:
  ```javascript
  const envDir = process.env.CLAUDE_CONFIG_DIR;
  if (envDir && fs.existsSync(path.join(envDir, 'get-shit-done', 'VERSION'))) {
    return envDir;
  }
  ```

**Process Management:**
- stdin timeout guards to prevent hanging
- Proper cleanup and exit handling
- windowsHide flag for detached processes on Windows

---

*Convention analysis: 2026-03-18*
