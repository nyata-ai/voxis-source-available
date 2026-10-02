package port

// ReadContext labels retained generic read guards. The OSS target has no
// special recording workspace, but the normal HTTP and MCP labels keep shared
// error handling explicit.
type ReadContext string

// Read contexts label the normal API and MCP readers retained by OSS.
const (
	ReadContextRegular ReadContext = "regular"
	ReadContextMCP     ReadContext = "mcp"
)
