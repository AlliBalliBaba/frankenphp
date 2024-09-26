#include "_cgo_export.h"
#include "watcher-c.h"

void handle_event(struct wtr_watcher_event event, void *data) {
  go_handle_file_watcher_event((char *)event.path_name, event.effect_type,
                               event.path_type, (uintptr_t)data);
}

uintptr_t start_new_watcher(char const *const path, uintptr_t data) {
  return (uintptr_t)wtr_watcher_open(path, handle_event, (void *)data);
}

int stop_watcher(uintptr_t watcher) {
  if (!wtr_watcher_close((void *)watcher)) {
    return 0;
  }
  return 1;
}
