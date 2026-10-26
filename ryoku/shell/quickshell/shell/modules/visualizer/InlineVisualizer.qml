pragma ComponentBehavior: Bound
import QtQuick
import "Singletons"

Item {
    id: root

    Repeater {
        model: Performance.visualizerResident && Config.enabled ? Config.count : 0
        delegate: VisualizerView {
            id: vizView
            anchors.fill: parent
            cfg: VizItem { data: Config.dataAt(vizView.index) }
        }
    }
}
