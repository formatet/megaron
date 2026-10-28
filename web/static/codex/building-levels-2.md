If cancelling a queued build is refused, the reason appears in the Buildings tab.

The construction queue has a fixed limit, shown in `keryx build --queue`; population and building upgrades do not add slots. Finish or cancel a queued building before starting another when it is full. Hex buildings use `place`, not `staff`: for a lumbermill use `keryx place timber <hex> +2` (cedar on a cedar hex). `build --hex` accepts coordinates such as `3,-1` or the City ordinal `6`/`#6`.
