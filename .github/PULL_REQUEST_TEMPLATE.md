## Summary

<!-- What does this PR do? 1-3 bullet points. -->

## Test plan

- [ ] `make test` passes
- [ ] `make vet` passes
- [ ] New/changed behavior has test coverage
- [ ] Manually tested the workflow end-to-end

## Security checklist (if touching `internal/vault/`, `internal/keyring/`, or `internal/llm/`)

- [ ] No secrets in CLI arguments, logs, or error messages
- [ ] `*memguard.LockedBuffer` lifecycle is correct (caller owns, must `Destroy()`)
- [ ] Nonces generated from `crypto/rand`, never deterministic
- [ ] Vault writes are atomic (temp file + rename)
