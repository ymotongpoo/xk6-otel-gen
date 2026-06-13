// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

// FindServiceByName returns the service for id if present.
func (s *Schema) FindServiceByName(id ServiceID) (*Service, bool) {
	svc, ok := s.Services[id]
	return svc, ok
}

// JourneyNames returns journey names in ascending order.
func (s *Schema) JourneyNames() []string {
	return sortedKeys(s.Journeys)
}
