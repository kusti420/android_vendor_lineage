# SPDX-FileCopyrightText: 2024 The LineageOS Project
# SPDX-License-Identifier: Apache-2.0

$(call inherit-product, device/google/cuttlefish/vsoc_x86_64/phone/aosp_cf.mk)

include vendor/lineage/build/target/product/lineage_generic_target.mk

TARGET_DISABLE_EPPE := true
TARGET_NO_KERNEL_OVERRIDE := true

# IchthysOS: the a52sxq bringup synced vendor/qcom/opensource/interfaces, whose modules
# live in the DEFAULT soong namespace and import QTI interfaces (vendor.qti.hardware.
# display.config/mapper/...) that live in QCOM namespaces. Soong analyses default-namespace
# modules for every product, so this Cuttlefish build fails those imports unless the QCOM
# display namespaces are registered. Register them here (parse-only; nothing QCOM is
# installed into the Cuttlefish image unless referenced) so shared SystemUI/framework work
# can be developed and tested on Cuttlefish.
PRODUCT_SOONG_NAMESPACES += \
    vendor/qcom/opensource/commonsys-intf/display \
    vendor/qcom/opensource/display

# Overrides
PRODUCT_NAME := lineage_cf_phone_x86_64
PRODUCT_MODEL := LineageOS Cuttlefish phone built for x86_64
