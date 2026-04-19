/* DUKTO - A simple, fast and multi-platform file transfer tool for LAN users
 * Copyright (C) 2021 Xu Zhen
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111-1307, USA.
 */

import QtQuick 2.14

Rectangle {
    id: container
    implicitWidth: checkbox.implicitWidth + label.anchors.leftMargin + label.implicitWidth
    implicitHeight: Math.max(checkbox.implicitHeight, label.implicitHeight + label.anchors.topMargin)
    color: "transparent"

    property bool checked: false
    property alias text: label.text

    signal clicked(bool checked)

    Rectangle {
        id: checkbox
        implicitWidth: 40
        implicitHeight: 24
        radius: height / 2
        color: theme.lighterBgColor
        border.color: checkboxArea.containsMouse ? theme.dimmedTextColor : theme.borderColor
        border.width: 2

        Rectangle {
            id: indicator
            anchors.top: parent.top
            color: checked ? theme.themeColor : theme.bgColor
            height: parent.height
            width: height
            radius: height / 2
            x: checked ? handler.xAxis.maximum : handler.xAxis.minimum
            border.color: parent.border.color
            border.width: 2

            Behavior on x { SmoothedAnimation { duration: 200 } }
            Behavior on color { ColorAnimation { duration: 100 } }
        }

        MouseArea {
            id: checkboxArea
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: containsMouse ? Qt.PointingHandCursor : Qt.ArrowCursor
            Connections {
                function onClicked() {
                    checked = !checked;
                    clicked(checked)
                }
            }
        }

        DragHandler {
            id: handler
            target: indicator
            yAxis.enabled: false
            xAxis.enabled: true
            xAxis.maximum: checkbox.width - indicator.width
            xAxis.minimum: 0
            onActiveChanged: {
                if (!active) {
                    if (indicator.x >= (xAxis.maximum - xAxis.minimum) / 2) {
                        indicator.x = xAxis.maximum
                        checked = true
                        clicked(true)
                    } else {
                        indicator.x = xAxis.minimum
                        checked = false
                        clicked(false)
                    }
                }
            }
        }
    }

    SText {
        id: label
        font.pixelSize: 16
        color: theme.textColor
        anchors.top: checkbox.top
        anchors.left: checkbox.right
        anchors.leftMargin: 4
        anchors.topMargin: 2

        MouseArea {
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: containsMouse ? Qt.PointingHandCursor : Qt.ArrowCursor
            Connections {
                function onClicked() {
                    checked = !checked;
                    clicked(checked)
                }
            }
        }
    }
}
