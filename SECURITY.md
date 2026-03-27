# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Vial, **please do not open a public issue.** Instead, email security concerns to the maintainers privately.

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Threat Model

This threat model is published for transparency, following the example set by [age](https://github.com/FiloSottile/age) and [1Password](https://1password.com/security).

### What Vial Protects Against

| Threat | Protection |
|--------|------------|
| Disk theft without master password | Vault file encrypted at rest with AES-256-GCM |
| Casual file browsing | Secrets never stored in plaintext on disk (except in generated `.env` files) |
| Accidental git commits of the vault | Vault stored in `~/.local/share/vial/`, not in project directories |
| Shell history leaks | Secret values never accepted as positional CLI arguments — always via stdin prompt or pipe |
| Backup exposure | Vault file is encrypted; backups contain only ciphertext |
| Swap file exposure | `mlock()` via memguard prevents secret memory pages from being swapped to disk |
| Brute-force password attacks | Argon2id with 64 MiB memory, 3 iterations targets 200–500ms derivation time |

### What Vial Does NOT Protect Against

| Threat | Reason |
|--------|--------|
| Active malware with root/admin access | Nothing in userspace can defend against a compromised kernel |
| Physical keyloggers + disk access | Master password capture bypasses all encryption |
| Compromised build toolchain | Supply-chain attacks on dependencies are out of scope |
| Memory forensics during active use | Secrets exist briefly in plaintext in guarded memory while the vault is unlocked |
| Plaintext `.env` files | Generated `.env` files contain plaintext secrets on disk — this is by design |

### Accepted Risks (Explicitly Communicated)

These are trade-offs that users accept when using Vial:

- **Key reuse blast radius.** A single vault compromise exposes all projects. Users accept this for workflow speed.
- **Plaintext `.env` on disk.** Generated `.env` files are plaintext. Users are responsible for `.gitignore` and full-disk encryption (FileVault / LUKS).
- **Master password strength.** Vault security is bounded by master password entropy. The tool enforces a minimum of 12 characters and recommends passphrases.
- **Session caching.** The DEK is cached in the OS keychain with a TTL. A compromised keychain during an active session exposes the DEK.

## Encryption Architecture

```
Master Password
  → Argon2id (m=64 MiB, t=3, p=1, salt=16 bytes random)
  → 256-bit Key Encryption Key (KEK)
  → Encrypts per-vault Data Encryption Key (DEK)
  → DEK encrypts individual secret values
  → via AES-256-GCM (12-byte random nonce per value)
```

### KDF Parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Algorithm | Argon2id | OWASP first choice; memory-hard + side-channel resistant |
| Memory | 64 MiB | Above OWASP minimum (46 MiB) |
| Iterations | 3 | Above OWASP minimum; 200–500ms derivation target |
| Parallelism | 1 | Conservative; avoids timing side channels |
| Salt | 16 bytes random | Per-vault unique |
| Output | 32 bytes (256-bit) | AES-256 key size |

### Nonce Management

AES-GCM nonces are always generated from `crypto/rand`. A fresh 12-byte nonce is generated for every encryption operation, even when overwriting an existing key. Nonces are never derived deterministically.

### Vault File Format

SOPS-inspired value-level encryption: key names remain readable (enabling meaningful diffs if the vault is version-controlled), while values are individually encrypted with unique nonces.

## Security Best Practices for Users

- Enable full-disk encryption (FileVault on macOS, LUKS on Linux)
- Use scoped/restricted API keys when providers support them
- Use a strong, unique master password — passphrases recommended
- Rotate keys periodically using `vial key set` + `vial pour --all`
- Keep the vault file out of cloud-synced directories unless intentional
- Add `.env` to every project's `.gitignore`

## Dependencies

Security-critical dependencies:

| Package | Purpose | Audit Status |
|---------|---------|-------------|
| `golang.org/x/crypto/argon2` | Key derivation | Go standard extended library |
| `crypto/aes`, `crypto/cipher` | AES-256-GCM encryption | Go standard library |
| `crypto/rand` | Nonce and key generation | Go standard library |
| `github.com/awnumar/memguard` | Secure memory (mlock, guarded heap) | Widely used, open source |
| `github.com/zalando/go-keyring` | OS keychain access | Widely used, open source |

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest `main` | Yes |
| Tagged releases | Yes |
