# freezecheck

`freezecheck` is the local, deterministic validator for experiment freeze,
schedule, and preserved run artifacts.

```text
go run . config --root ../..
go run . schedule generate --config ../../config/experiment.json --phase measured \
  --seed SEED --config-revision REV --generated-at 2026-08-18T12:00:00Z \
  --output ../../config/measured-schedule.json
go run . schedule validate --config ../../config/experiment.json \
  --schedule ../../config/measured-schedule.json --phase measured
go run . run --root ../.. --run-dir /path/to/run \
  --schedule ../../config/measured-schedule.json
go run . results --root ../.. --results-dir /path/to/results \
  --schedule ../../config/measured-schedule.json
```

Generation has no clock or random defaults. Manifest algorithm
`sha256-fisher-yates-v1` uses an architecture-independent byte
stream and rejection-sampled Fisher-Yates shuffle are specified in
`schedule.go`. Output publication is atomic and refuses replacement.

Image checks prove digest shape and agreement between local metadata files;
they do not claim an independent OCI build or registry verification. One-run
validation rejects replacement links; `results` validates reciprocal links,
same-cell replacement, uniqueness, and resolution across a complete local set.
