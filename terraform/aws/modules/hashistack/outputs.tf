output "server_public_ips" {
  value = aws_instance.server[*].public_ip
}

output "client_public_ips" {
  value = aws_instance.client[*].public_ip
}

output "server_lb_ip" {
  value = aws_elb.server_lb.dns_name
}

output "client_lb_ip" {
  value = aws_elb.client_lb.dns_name
}
