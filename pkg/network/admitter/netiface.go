/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2024 Red Hat, Inc.
 *
 */

package admitter

import (
	"fmt"
	"net"
	"regexp"

	"kubevirt.io/kubevirt/pkg/network/vmispec"
	hwutil "kubevirt.io/kubevirt/pkg/util/hardware"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"

	v1 "kubevirt.io/api/core/v1"
)

func validateNetworksAssignedToInterfaces(field *k8sfield.Path, spec *v1.VirtualMachineInstanceSpec) []metav1.StatusCause {
	var causes []metav1.StatusCause
	const nameOfTypeNotFoundMessagePattern = "%s '%s' not found."
	interfaceSet := vmispec.IndexInterfaceSpecByName(spec.Domain.Devices.Interfaces)
	for i, network := range spec.Networks {
		if _, exists := interfaceSet[network.Name]; !exists {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueRequired,
				Message: fmt.Sprintf(nameOfTypeNotFoundMessagePattern, field.Child("networks").Index(i).Child("name").String(), network.Name),
				Field:   field.Child("networks").Index(i).Child("name").String(),
			})
		}
	}
	return causes
}

func validateInterfacesAssignedToNetworks(field *k8sfield.Path, spec *v1.VirtualMachineInstanceSpec) []metav1.StatusCause {
	var causes []metav1.StatusCause
	const nameOfTypeNotFoundMessagePattern = "%s '%s' not found."
	networkSet := vmispec.IndexNetworkSpecByName(spec.Networks)
	for idx, iface := range spec.Domain.Devices.Interfaces {
		if _, exists := networkSet[iface.Name]; !exists {
			causes = append(causes, metav1.StatusCause{
				Type: metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf(
					nameOfTypeNotFoundMessagePattern,
					field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
					iface.Name,
				),
				Field: field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
			})
		}
	}
	return causes
}

func validateNetworkNameUnique(field *k8sfield.Path, spec *v1.VirtualMachineInstanceSpec) []metav1.StatusCause {
	var causes []metav1.StatusCause
	networkSet := map[string]struct{}{}
	for i, network := range spec.Networks {
		if _, exists := networkSet[network.Name]; exists {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueDuplicate,
				Message: fmt.Sprintf("Network with name %q already exists, every network must have a unique name", network.Name),
				Field:   field.Child("networks").Index(i).Child("name").String(),
			})
		}
		networkSet[network.Name] = struct{}{}
	}
	return causes
}

func validateInterfaceNameUnique(field *k8sfield.Path, spec *v1.VirtualMachineInstanceSpec) []metav1.StatusCause {
	var causes []metav1.StatusCause
	ifaceSet := map[string]struct{}{}
	for idx, iface := range spec.Domain.Devices.Interfaces {
		if _, exists := ifaceSet[iface.Name]; exists {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueDuplicate,
				Message: "Only one interface can be connected to one specific network",
				Field:   field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
			})
		}
		ifaceSet[iface.Name] = struct{}{}
	}
	return causes
}

func validateInterfacesFields(field *k8sfield.Path, spec *v1.VirtualMachineInstanceSpec) []metav1.StatusCause {
	var causes []metav1.StatusCause
	for idx, iface := range spec.Domain.Devices.Interfaces {
		causes = append(causes, validateInterfaceNameFormat(field, idx, iface)...)
		causes = append(causes, validateInterfaceModel(field, idx, iface)...)
		causes = append(causes, validateMacAddress(field, idx, iface)...)
		causes = append(causes, validatePciAddress(field, idx, iface)...)
	}
	return causes
}

func validateInterfaceNameFormat(field *k8sfield.Path, idx int, iface v1.Interface) []metav1.StatusCause {
	isValid := regexp.MustCompile(`^[A-Za-z0-9-_]+$`).MatchString
	if !isValid(iface.Name) {
		return []metav1.StatusCause{{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: "Network interface name can only contain alphabetical characters, numbers, dashes (-) or underscores (_)",
			Field:   field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
		}}
	}
	return nil
}

var validInterfaceModels = map[string]struct{}{
	"e1000":    {},
	"e1000e":   {},
	"ne2k_pci": {},
	"pcnet":    {},
	"rtl8139":  {},
	v1.VirtIO:  {},
}

func validateInterfaceModel(field *k8sfield.Path, idx int, iface v1.Interface) []metav1.StatusCause {
	if iface.Model != "" {
		if _, exists := validInterfaceModels[iface.Model]; !exists {
			return []metav1.StatusCause{{
				Type: metav1.CauseTypeFieldValueNotSupported,
				Message: fmt.Sprintf(
					"interface %s uses model %s that is not supported.",
					field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
					iface.Model,
				),
				Field: field.Child("domain", "devices", "interfaces").Index(idx).Child("model").String(),
			}}
		}
	}
	return nil
}

func validateMacAddress(field *k8sfield.Path, idx int, iface v1.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	if iface.MacAddress != "" {
		mac, err := net.ParseMAC(iface.MacAddress)
		if err != nil {
			causes = append(causes, metav1.StatusCause{
				Type: metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf(
					"interface %s has malformed MAC address (%s).",
					field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
					iface.MacAddress,
				),
				Field: field.Child("domain", "devices", "interfaces").Index(idx).Child("macAddress").String(),
			})
		}
		const macLen = 6
		if len(mac) > macLen {
			causes = append(causes, metav1.StatusCause{
				Type: metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf(
					"interface %s has MAC address (%s) that is too long.",
					field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
					iface.MacAddress,
				),
				Field: field.Child("domain", "devices", "interfaces").Index(idx).Child("macAddress").String(),
			})
		}
	}
	return causes
}

func validatePciAddress(field *k8sfield.Path, idx int, iface v1.Interface) []metav1.StatusCause {
	if iface.PciAddress != "" {
		_, err := hwutil.ParsePciAddress(iface.PciAddress)
		if err != nil {
			return []metav1.StatusCause{{
				Type: metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf(
					"interface %s has malformed PCI address (%s).",
					field.Child("domain", "devices", "interfaces").Index(idx).Child("name").String(),
					iface.PciAddress,
				),
				Field: field.Child("domain", "devices", "interfaces").Index(idx).Child("pciAddress").String(),
			}}
		}
	}
	return nil
}
