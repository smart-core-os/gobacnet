package types

// RejectReason is the BACnetRejectReason sent in a Reject-PDU, clause 21.
type RejectReason uint8

const (
	RejectOther                    RejectReason = 0
	RejectBufferOverflow           RejectReason = 1
	RejectInconsistentParameters   RejectReason = 2
	RejectInvalidParameterDataType RejectReason = 3
	RejectInvalidTag               RejectReason = 4
	RejectMissingRequiredParameter RejectReason = 5
	RejectParameterOutOfRange      RejectReason = 6
	RejectTooManyArguments         RejectReason = 7
	RejectUndefinedEnumeration     RejectReason = 8
	RejectUnrecognizedService      RejectReason = 9
)

// AbortReason is the BACnetAbortReason sent in an Abort-PDU, clause 21.
type AbortReason uint8

const (
	AbortOther                         AbortReason = 0
	AbortBufferOverflow                AbortReason = 1
	AbortInvalidAPDUInThisState        AbortReason = 2
	AbortPreemptedByHigherPriorityTask AbortReason = 3
	AbortSegmentationNotSupported      AbortReason = 4
	AbortSecurityError                 AbortReason = 5
	AbortInsufficientSecurity          AbortReason = 6
	AbortWindowSizeOutOfRange          AbortReason = 7
	AbortApplicationExceededReplyTime  AbortReason = 8
	AbortOutOfResources                AbortReason = 9
	AbortTSMTimeout                    AbortReason = 10
	AbortAPDUTooLong                   AbortReason = 11
)
