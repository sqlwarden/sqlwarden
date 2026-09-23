// Package audit defines durable security and business audit contracts.
//
// An audit event is part of the use case that produced it, not a transport
// concern: application services emit events, and transports map requests and
// responses only. Editions extend recording by decorating [Writer]; a
// decorator may add tamper evidence or export, but it may never weaken the
// durability guarantee of the core writer it wraps.
package audit
