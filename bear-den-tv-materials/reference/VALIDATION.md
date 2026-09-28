# Bundle validation

These checks validate the written design bundle, not a running product.

- The example JSON passed its included Draft 2020-12 schema.
- Negative schema cases rejected: enabled LAN without selected interface, HTTPS enabled without certificate paths, pairing expires beyond allowed bound, unexpected command field, enabled Plex connector without connection reference.
- Example section references and unique section IDs were checked.
- Main-document source IDs and Markdown code-fence balance were checked.
- No software was installed on the user’s mini PC and no hardware/client acceptance tests were executed.
