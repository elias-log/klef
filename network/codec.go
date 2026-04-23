// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package network

/*
   [TODO: Performance & Reliability Refinement]

   1. Chunking (Message Segmentation):
      - Currently, FetchResponse bundles all requested vertices into a single message.
      - Large-scale DAGs with hundreds of dependencies risk exceeding the Maximum Transmission Unit (MTU).
      - We must implement a MaxPayloadSize (e.g., 1MB) and split oversized responses into multiple segments.

   2. Prioritization (Bandwidth Optimization):
      - During high-traffic fetch scenarios, prioritize vertices from recent rounds (CurrentRound - 1).
      - Lower the transmission priority for historical data to ensure the tip of the DAG remains synchronized.
*/

import (
	"bytes"
	"encoding/gob"
)

/// Encode serializes a generic object into a byte slice using the GOB format.
func Encode(msg interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

/// Decode deserializes a byte slice back into the provided target object.
func Decode(data []byte, target interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(target)
}
