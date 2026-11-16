//go:build darwin || linux || windows

package owner

// service is the name the key is kept under in an OS store (ServiceName,
// which the advice for a removed seal spells out on every platform).
const service = ServiceName
