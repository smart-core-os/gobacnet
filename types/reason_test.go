package types

import (
	"testing"

	"github.com/smart-core-os/gobacnet/enum/errorcode"
)

func TestRejectReason_ErrorCode(t *testing.T) {
	tests := []struct {
		reason RejectReason
		want   errorcode.ErrorCode
	}{
		{RejectOther, errorcode.RejectOther},
		{RejectBufferOverflow, errorcode.RejectBufferOverflow},
		{RejectInconsistentParameters, errorcode.RejectInconsistentParameters},
		{RejectInvalidParameterDataType, errorcode.RejectInvalidParameterDataType},
		{RejectInvalidTag, errorcode.RejectInvalidTag},
		{RejectMissingRequiredParameter, errorcode.RejectMissingRequiredParameter},
		{RejectParameterOutOfRange, errorcode.RejectParameterOutOfRange},
		{RejectTooManyArguments, errorcode.RejectTooManyArguments},
		{RejectUndefinedEnumeration, errorcode.RejectUndefinedEnumeration},
		{RejectUnrecognizedService, errorcode.RejectUnrecognizedService},
		{10, errorcode.RejectOther}, // reserved
		{63, errorcode.RejectOther},
		{64, errorcode.RejectProprietary},
		{255, errorcode.RejectProprietary},
	}
	for _, tt := range tests {
		if got := tt.reason.ErrorCode(); got != tt.want {
			t.Errorf("%v.ErrorCode() = %v, want %v", tt.reason, got, tt.want)
		}
	}
}

func TestAbortReason_ErrorCode(t *testing.T) {
	tests := []struct {
		reason AbortReason
		want   errorcode.ErrorCode
	}{
		{AbortOther, errorcode.AbortOther},
		{AbortBufferOverflow, errorcode.AbortBufferOverflow},
		{AbortInvalidAPDUInThisState, errorcode.AbortInvalidApduInThisState},
		{AbortPreemptedByHigherPriorityTask, errorcode.AbortPreemptedByHigherPriorityTask},
		{AbortSegmentationNotSupported, errorcode.AbortSegmentationNotSupported},
		{AbortSecurityError, errorcode.AbortSecurityError},
		{AbortInsufficientSecurity, errorcode.AbortInsufficientSecurity},
		{AbortWindowSizeOutOfRange, errorcode.AbortWindowSizeOutOfRange},
		{AbortApplicationExceededReplyTime, errorcode.AbortApplicationExceededReplyTime},
		{AbortOutOfResources, errorcode.AbortOutOfResources},
		{AbortTSMTimeout, errorcode.AbortTsmTimeout},
		{AbortAPDUTooLong, errorcode.AbortApduTooLong},
		{12, errorcode.AbortOther}, // reserved
		{63, errorcode.AbortOther},
		{64, errorcode.AbortProprietary},
		{255, errorcode.AbortProprietary},
	}
	for _, tt := range tests {
		if got := tt.reason.ErrorCode(); got != tt.want {
			t.Errorf("%v.ErrorCode() = %v, want %v", tt.reason, got, tt.want)
		}
	}
}

func TestReason_String(t *testing.T) {
	for _, tt := range []struct{ got, want string }{
		{RejectUnrecognizedService.String(), "unrecognized-service"},
		{RejectReason(70).String(), "proprietary(70)"},
		{AbortSegmentationNotSupported.String(), "segmentation-not-supported"},
		{AbortAPDUTooLong.String(), "apdu-too-long"},
		{AbortReason(20).String(), "unknown(20)"},
	} {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}
