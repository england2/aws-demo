#!/usr/bin/env fish

set script_dir (cd (dirname (status --current-filename)); pwd)

env \
    FAUX_EVENT_TIME="2026-05-01T04:15:44Z" \
    FAUX_QUERY_DATE="2026-05-01T04:15:44.090+0000" \
    FAUX_START_DATE="2026-05-01T04:15:00.000+0000" \
    FAUX_STATE_TIMESTAMP="2026-05-01T04:15:44.091+0000" \
    FAUX_PREVIOUS_QUERY_DATE="2026-05-01T04:11:54.091+0000" \
    FAUX_PREVIOUS_STATE_TIMESTAMP="2026-05-01T04:11:54.092+0000" \
    FAUX_ALARM_NAME="debian-cpu-spin-high-cpu" \
    FAUX_STATE_VALUE="ALARM" \
    FAUX_PREVIOUS_STATE_VALUE="INSUFFICIENT_DATA" \
    FAUX_INSTANCE_ID="i-03f8306225046aca5" \
    FAUX_METRIC_ID="e6ceefb7-f504-ebf4-035d-e13798e92d3f" \
    FAUX_DATAPOINT="99.83333333333333" \
    FAUX_PREVIOUS_DATAPOINT="" \
    FAUX_THRESHOLD="20.0" \
    FAUX_PERIOD="20" \
    FAUX_REASON="Threshold Crossed: 1 datapoint [99.83333333333333 (01/05/26 04:15:00)] was greater than the threshold (20.0)." \
    FAUX_PREVIOUS_REASON="Insufficient Data: 1 datapoint was unknown." \
    FAUX_DESCRIPTION="Triggers when debian-cpu-spin CPU is above 70 percent for 1 minute" \
    fish "$script_dir/push-faux-sqs-alert.fish"
