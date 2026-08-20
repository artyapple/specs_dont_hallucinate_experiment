# Treatment Overlays

Treatment overlays change only generator availability and the corresponding handwritten/generated boundaries.

Common business requirements, generic guidance, model, budget, permissions, network policy, non-treatment application dependencies, caches, and visible-check purpose remain identical. Generator binaries and configurations are the treatment-specific exception.

Overlay revisions are frozen before measured runs.

For Greenfield Codegen, `codegen/workspace/` is copied over Base 1 before the
agent starts. It adds only pinned dependencies, generator configuration, and
canonical generation commands; it contains no generated service implementation.
