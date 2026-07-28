# Lazy inventory plan

- [x] Keep startup discovery limited to enumerating and parsing `.kcpps` files; defer hashing referenced model assets.
- [x] Return lightweight node inventory by default, without recursively walking `models.file_roots`.
- [x] Add an explicit full-inventory request used when the Models tab is opened, and propagate that choice from the master to slaves.
- [x] Log deferred catalog hashing and file scans, including failures and elapsed time, through the runtime logger so `logging.mode` controls them.
- [x] Add focused backend tests proving startup/node discovery stays lightweight while the Models tab requests full inventory.
- [x] Run Go tests plus WebUI lint/tests/build, then update user documentation.
