# tools/

Local decomk corpus-maintenance tools.

| dir | purpose |
| --- | --- |
| `mint-handle/` | Mint one fresh proquint handle for a new TODO, TE, or DI record. |
| `migrate-handles/` | One-time migration from legacy numeric/timestamp IDs to proquint IDs. |
| `sweep-citations/` | Rewrite current legacy references using `migrate-handles/mapping.tsv`. |

## Minting a new handle

```bash
cd tools/mint-handle
go run . -r ../..
```

The command prints only the handle, for example `vapoj`. Prefix it with the
record kind: `TODO-vapoj`, `TE-vapoj`, or `DI-vapoj`.

## Migration authority

`tools/migrate-handles/mapping.tsv` is the machine-readable authority for the
legacy-to-proquint migration. Root `numeric-proquint-xref.md` is generated from
that TSV for human lookup.
