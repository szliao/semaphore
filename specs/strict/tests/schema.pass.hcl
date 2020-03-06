service "caller" "http" "json" {
	host = ""
	schema = "caller"
}

flow "echo" {
  input "input" {
	}

	call "opening" {
		request "caller" "Open" {
			message = "{{ input:message }}"
		}
	}

	output "output" {
		message = "{{ input:message }}"
	}
}