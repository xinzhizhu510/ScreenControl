#include <CoreGraphics/CoreGraphics.h>
#include <stdbool.h>
#include <stddef.h>

void dc_display_info(CGDirectDisplayID id, char *uuid, size_t uuidSize, char *name, size_t nameSize);
void dc_identify_screen(CGDirectDisplayID id, unsigned int number);
bool dc_keep_window_visible(CGDirectDisplayID id);
bool dc_can_disconnect(CGDirectDisplayID id);
CGError dc_set_display_connected(CGDirectDisplayID id, bool connected);
