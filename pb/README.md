# go-judge protobuf package

## Migration

Following the [Blog](https://go.dev/blog/protobuf-opaque) opaque API migration the pb package will be adapted to newer API for the future. 

Although The client integration should stays the same giving the backwards compatibility promised by the protobuf team, it is still recommended to migrate to newer version.

[FAQ](https://protobuf.dev/reference/go/opaque-faq/)

For clients with older API, it is recommended to stick with `v1.0.1` for now.

To migration to newer version:

- Upgrade to `v1.1.0`
- Following the [migration guide](https://protobuf.dev/reference/go/opaque-migration/) to install the `open2opaque` tool
- Use the tool to migrate existing code to use newer version
- Upgrade to `v1.2.0` to finish the migration
