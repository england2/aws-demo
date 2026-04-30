resource "aws_cloudwatch_metric_alarm" "cpu_spin_high_cpu" {
  alarm_name          = "debian-cpu-spin-high-cpu"
  alarm_description   = "Triggers when debian-cpu-spin CPU is above 70 percent for 1 minute"
  comparison_operator = "GreaterThanThreshold"
  # the number of windows/periods that must breach the threshold for an alert to occur
  evaluation_periods = 1
  metric_name        = "CPUUtilization"
  namespace          = "AWS/EC2"
  # period: the size of the metric aggregation window being evaluated
  # here, this means "if in the last 20 seconds..."
  period = 20
  # "... the average CPU utilization is above..."
  statistic = "Average"
  # "... 20%, then trigger an alarm"
  threshold = 20

  dimensions = {
    InstanceId = aws_instance.server.id
  }
}
