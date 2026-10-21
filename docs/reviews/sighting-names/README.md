# Sighting dispatches use canonical unit labels

Contract: ForeignMarchSighted/V2 show the existing unit naming owner's player label, including stored older notifications: merchantman → Emporos. Preserve raw payloads, sighting semantics, privacy and the player's ship names. Scope web/CLI formatters and tests. No new name system, event type, layout or mechanics. TEXT; player reported the actual dispatch. Both versions covered; V2 still withholds destination, existing unknown-type fallback remains. Targeted Keryx and JS format suites GREEN. Existing central naming owner tests continue to define the labels. No persistence change.

Rebased on bf950127:687JS GREEN, Keryx tests and vet GREEN. Literal raw-type mutation named RED in the saved-sighting regression, restored formatter GREEN. Existing event payloads and ship names remain unchanged. No BILD change; TEXT to verify in the reported old dispatch after reload.
