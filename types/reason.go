package types

import (
	"fmt"

	"github.com/smart-core-os/gobacnet/enum/errorcode"
)

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

// reasonProprietary is the first reason value reserved for vendor use, clause 21.
const reasonProprietary = 64

var rejectReasonNames = [...]string{
	"other", "buffer-overflow", "inconsistent-parameters", "invalid-parameter-data-type", "invalid-tag",
	"missing-required-parameter", "parameter-out-of-range", "too-many-arguments", "undefined-enumeration",
	"unrecognized-service",
}

func (r RejectReason) String() string {
	if int(r) < len(rejectReasonNames) {
		return rejectReasonNames[r]
	}
	if r >= reasonProprietary {
		return fmt.Sprintf("proprietary(%d)", uint8(r))
	}
	return fmt.Sprintf("unknown(%d)", uint8(r))
}

// ErrorCode returns the error code a Reject with this reason is reported as.
func (r RejectReason) ErrorCode() errorcode.ErrorCode {
	switch r {
	case RejectBufferOverflow:
		return errorcode.RejectBufferOverflow
	case RejectInconsistentParameters:
		return errorcode.RejectInconsistentParameters
	case RejectInvalidParameterDataType:
		return errorcode.RejectInvalidParameterDataType
	case RejectInvalidTag:
		return errorcode.RejectInvalidTag
	case RejectMissingRequiredParameter:
		return errorcode.RejectMissingRequiredParameter
	case RejectParameterOutOfRange:
		return errorcode.RejectParameterOutOfRange
	case RejectTooManyArguments:
		return errorcode.RejectTooManyArguments
	case RejectUndefinedEnumeration:
		return errorcode.RejectUndefinedEnumeration
	case RejectUnrecognizedService:
		return errorcode.RejectUnrecognizedService
	}
	if r >= reasonProprietary {
		return errorcode.RejectProprietary
	}
	return errorcode.RejectOther
}

var abortReasonNames = [...]string{
	"other", "buffer-overflow", "invalid-apdu-in-this-state", "preempted-by-higher-priority-task",
	"segmentation-not-supported", "security-error", "insufficient-security", "window-size-out-of-range",
	"application-exceeded-reply-time", "out-of-resources", "tsm-timeout", "apdu-too-long",
}

func (r AbortReason) String() string {
	if int(r) < len(abortReasonNames) {
		return abortReasonNames[r]
	}
	if r >= reasonProprietary {
		return fmt.Sprintf("proprietary(%d)", uint8(r))
	}
	return fmt.Sprintf("unknown(%d)", uint8(r))
}

// ErrorCode returns the error code an Abort with this reason is reported as.
func (r AbortReason) ErrorCode() errorcode.ErrorCode {
	switch r {
	case AbortBufferOverflow:
		return errorcode.AbortBufferOverflow
	case AbortInvalidAPDUInThisState:
		return errorcode.AbortInvalidApduInThisState
	case AbortPreemptedByHigherPriorityTask:
		return errorcode.AbortPreemptedByHigherPriorityTask
	case AbortSegmentationNotSupported:
		return errorcode.AbortSegmentationNotSupported
	case AbortSecurityError:
		return errorcode.AbortSecurityError
	case AbortInsufficientSecurity:
		return errorcode.AbortInsufficientSecurity
	case AbortWindowSizeOutOfRange:
		return errorcode.AbortWindowSizeOutOfRange
	case AbortApplicationExceededReplyTime:
		return errorcode.AbortApplicationExceededReplyTime
	case AbortOutOfResources:
		return errorcode.AbortOutOfResources
	case AbortTSMTimeout:
		return errorcode.AbortTsmTimeout
	case AbortAPDUTooLong:
		return errorcode.AbortApduTooLong
	}
	if r >= reasonProprietary {
		return errorcode.AbortProprietary
	}
	return errorcode.AbortOther
}
