//go:build !moq

package cloud

import (
	"context"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

// StartLiveStreamMoQ is disabled in the standard Agent build.
func StartLiveStreamMoQ(
	_ context.Context,
	_ *models.Configuration,
	_ *models.Communication,
	_ bool,
	_ *packets.Queue,
	_ *packets.Queue,
) {
}
