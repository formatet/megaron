# E5 JavaScript — remove unused declarations and exports

## Contract
- Problem: unused JavaScript declarations and file-private functions presented as exports obscure the supported module API.
- Player truth: no rendered content, behavior, import consumer or test capability changes.
- Invariant: keep anything reached by static, namespace, dynamic, HTML, test or tool consumers; remove declarations only if their initializers are side-effect-free.
- Scope: audited JS declarations/exports and this report.
- Non-scope: CSS, Go, gameplay, broad stylistic refactoring.
- Acceptance: inventory identifies each changed symbol; web tests green; native module linking holds; meaningful retained-export mutant fails.
- Stop: ambiguous dynamic consumers or behavior changes; retain and explain instead.
- Proof: full-repository name search, module use audit, before/after web tests, module linker probe, source diff.

Gate: infrastructure support for the chain; no player surface changes. Parent runs full fresh-DB baseline and final suite.
