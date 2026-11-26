#import <AppKit/AppKit.h>
#import <dlfcn.h>
#import "native_darwin.h"

static NSScreen *screenForDisplay(CGDirectDisplayID displayID) {
    for (NSScreen *screen in [NSScreen screens]) {
        if ([screen.deviceDescription[@"NSScreenNumber"] unsignedIntValue] == displayID) {
            return screen;
        }
    }
    return nil;
}

typedef CGError (*ConfigureDisplayEnabled)(CGDisplayConfigRef, CGDirectDisplayID, bool);

static ConfigureDisplayEnabled configureDisplayEnabled(void) {
    return (ConfigureDisplayEnabled)dlsym(RTLD_DEFAULT, "CGSConfigureDisplayEnabled");
}

bool dc_can_disconnect(CGDirectDisplayID id) {
    return configureDisplayEnabled() != NULL && !CGDisplayIsInMirrorSet(id);
}

CGError dc_set_display_connected(CGDirectDisplayID id, bool connected) {
    ConfigureDisplayEnabled configure = configureDisplayEnabled();
    if (configure == NULL) return kCGErrorNotImplemented;

    CGDisplayConfigRef config;
    CGError status = CGBeginDisplayConfiguration(&config);
    if (status != kCGErrorSuccess) return status;

    status = configure(config, id, connected);
    if (status != kCGErrorSuccess) {
        CGCancelDisplayConfiguration(config);
        return status;
    }
    return CGCompleteDisplayConfiguration(config, kCGConfigureForSession);
}

bool dc_keep_window_visible(CGDirectDisplayID displayID) {
    __block bool ready = false;
    void (^moveWindow)(void) = ^{
        NSWindow *window = NSApp.mainWindow;
        NSScreen *destination = nil;
        for (NSScreen *screen in [NSScreen screens]) {
            CGDirectDisplayID candidate = [screen.deviceDescription[@"NSScreenNumber"] unsignedIntValue];
            if (candidate == displayID || !CGDisplayIsActive(candidate) || CGDisplayIsAsleep(candidate) ||
                CGDisplayMirrorsDisplay(candidate) == displayID || CGDisplayMirrorsDisplay(displayID) == candidate) {
                continue;
            }
            if (screen == window.screen) {
                ready = true;
                return;
            }
            if (destination == nil || CGDisplayIsBuiltin(candidate)) destination = screen;
        }
        if (window == nil || destination == nil || (window.styleMask & NSWindowStyleMaskFullScreen)) return;

        NSRect bounds = destination.visibleFrame;
        NSRect frame = window.frame;
        frame.size.width = MIN(NSWidth(frame), NSWidth(bounds));
        frame.size.height = MIN(NSHeight(frame), NSHeight(bounds));
        frame.origin = NSMakePoint(NSMidX(bounds) - NSWidth(frame) / 2,
                                   NSMidY(bounds) - NSHeight(frame) / 2);
        [window setFrame:frame display:YES];
        ready = window.screen == destination;
    };
    if ([NSThread isMainThread]) {
        moveWindow();
    } else {
        dispatch_sync(dispatch_get_main_queue(), moveWindow);
    }
    return ready;
}

void dc_display_info(CGDirectDisplayID displayID, char *uuid, size_t uuidSize, char *name, size_t nameSize) {
    CFUUIDRef displayUUID = CGDisplayCreateUUIDFromDisplayID(displayID);
    if (displayUUID != NULL) {
        CFStringRef string = CFUUIDCreateString(kCFAllocatorDefault, displayUUID);
        if (string != NULL) {
            CFStringGetCString(string, uuid, uuidSize, kCFStringEncodingUTF8);
            CFRelease(string);
        }
        CFRelease(displayUUID);
    }

    void (^copyName)(void) = ^{
        NSString *nameValue = screenForDisplay(displayID).localizedName;
        [nameValue getCString:name maxLength:nameSize encoding:NSUTF8StringEncoding];
    };
    if ([NSThread isMainThread]) {
        copyName();
    } else {
        dispatch_sync(dispatch_get_main_queue(), copyName);
    }
}

void dc_identify_screen(CGDirectDisplayID displayID, unsigned int number) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSScreen *screen = screenForDisplay(displayID);
        if (screen == nil) return;

        NSRect display = screen.visibleFrame;
        NSRect frame = NSMakeRect(NSMidX(display) - 53, NSMidY(display) - 39, 106, 78);
        NSPanel *panel = [[NSPanel alloc] initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
            backing:NSBackingStoreBuffered defer:NO screen:screen];
        [panel setFrame:frame display:NO];
        panel.releasedWhenClosed = NO;
        panel.level = NSFloatingWindowLevel;
        panel.ignoresMouseEvents = YES;
        panel.backgroundColor = [NSColor colorWithRed:0.08 green:0.17 blue:0.17 alpha:0.97];
        panel.opaque = NO;
        panel.hasShadow = YES;
        panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorFullScreenAuxiliary;

        NSTextField *text = [NSTextField labelWithString:[NSString stringWithFormat:@"%u", number]];
        text.font = [NSFont monospacedDigitSystemFontOfSize:42 weight:NSFontWeightSemibold];
        text.textColor = [NSColor colorWithRed:0.62 green:0.96 blue:0.85 alpha:1.0];
        text.alignment = NSTextAlignmentCenter;
        text.frame = NSMakeRect(0, 11, 106, 56);
        [panel.contentView addSubview:text];
        [panel orderFrontRegardless];

        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
            [panel close];
            [panel release];
        });
    });
}
