# KCPPS Sharing

A normal `.kcpps` file points to model files on one machine. Those paths usually do not exist on another machine. Portable export converts recognized model path fields into content-addressed references that can be shared without exposing the original directory layout.

## Portable reference format

For each recognized model field, export removes the local path and writes:

- `<field>_hash`: lowercase SHA-256 of the asset
- `<field>_filename`: the asset file name
- `<field>_hf`: an optional commit-pinned Hugging Face origin

The Hugging Face form is `hf://owner/repository@commit/path`. Export includes it only when the local artifact index has a valid repository, commit, and path for the asset. Fields that accept arrays retain matching hash, filename, and origin arrays.

Example shape:

```json
{
  "model_param_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "model_param_filename": "model.gguf",
  "model_param_hf": "hf://owner/repository@0123456789abcdef/model.gguf"
}
```

The hash above only illustrates the required format.

## Export and share

1. Add model directories to `models.file_roots` so the router can inventory and hash them.
2. If the files came from the downloader, rescan its library so repository and commit metadata are indexed.
3. In the WebUI Models tab, select the saved configuration and export it.
4. Copy the exported `.kcpps` file to the receiving router's `models.config_dir`, or apply it through the WebUI configuration controls.
5. Resolve its assets in the Models tab or load the model and let resolution run before backend startup.

The export API is `POST /router/v1/site/model-assets/export`. It can export a configuration on the local router or an authorized cluster node.

## Resolution order

For each hash reference, the receiving router checks:

1. Its indexed local assets
2. Files below `models.file_roots`
3. An available cluster peer
4. The config's commit-pinned Hugging Face origin
5. A previously indexed Hugging Face origin for that hash
6. A unique exact Hugging Face candidate found by the downloader

Local and peer files are accepted only after SHA-256 verification. A Hugging Face file is downloaded only when the repository revision resolves to the referenced commit and the repository file's LFS hash equals the `.kcpps` hash. The resolved local path then replaces the portable fields in the receiving copy of the configuration.

`models.shared_dir` is the destination for shared asset transfers. When it is empty, the router uses `cluster.store_dir/model-assets`.

## Limits and failures

- A portable field cannot contain both a local path and portable metadata.
- Hashes must be 64 lowercase hexadecimal characters.
- Filenames must be local file names without directory components or unsafe platform characters.
- Hugging Face origins must name a repository, a hexadecimal commit, and a repository-relative path.
- If an asset cannot be verified or retrieved, the field stays unresolved and model loading fails before the backend starts.
- Sharing a `.kcpps` file does not redistribute model files. The receiving user remains responsible for repository access and model license terms.

Use `models.concurrent_asset_transfers` to bound simultaneous cluster asset transfers and `models.hash_workers` to bound local hashing work. See [Configuration](Configuration).
