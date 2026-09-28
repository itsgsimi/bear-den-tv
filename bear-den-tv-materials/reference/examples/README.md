# Proposed configuration examples

These files describe an initial configuration subset for implementation; no Bear Den TV executable exists in this package.

`config.example.json` is an onboarding-safe example. The LAN listener and autostart start disabled until the user consents. Plex content rows start disabled until a separate content connection is configured. This does not reduce the priority of those features in the product.

The remote can be enabled after selecting a real interface. The example deliberately does not guess the name of the mini PC's interface. HTTPS requires actual configured certificate/key paths and browser trust. Never paste service secrets into this file.

`config.schema.json` validates the example's structure, bounds, and some conditional requirements. It is not a complete future product schema or security boundary. The coordinator must additionally validate identifiers, section references, app allowlists, launch arguments, interface ownership, certificate configuration, and capability availability. Remote clients must never be able to replace arbitrary launch definitions through this schema.

The example and schema were checked together using a JSON Schema Draft 2020-12 validator when this design bundle was prepared.
