proxy "" {
  error "proto.Error" {
    message = "{{ error:message }}"
    status  = "{{ error:status }}"

    message "nested" {
      message  "nested" {}
      repeated ""       ""       {}
    }

    repeated "" "" {
      message  "nested" {}
      repeated ""       ""       {}
    }
  }

  on_error {
    schema  = "com.Schema"
    status  = 401
    message = "node error message"
  }

  forward "" {}
}
