// MOE — client-side helpers
"use strict";

// ── Toast notification manager (Alpine.js component) ────────────────────
function toastManager() {
    return {
        toasts: [],
        _nextId: 0,

        init() {
            // Read flash messages from URL query params and show them.
            var params = new URLSearchParams(window.location.search);
            var msg = params.get("flash");
            var type = params.get("flash_type") || "success";
            if (msg) {
                this.show(msg, type);
                // Clean the URL without reloading.
                var url = new URL(window.location);
                url.searchParams.delete("flash");
                url.searchParams.delete("flash_type");
                window.history.replaceState({}, "", url);
            }
        },

        show(message, type) {
            type = type || "success";
            var id = ++this._nextId;
            this.toasts.push({ id: id, message: message, type: type, visible: true });
            var self = this;
            setTimeout(function() { self.dismiss(id); }, 5000);
        },

        dismiss(id) {
            var t = this.toasts.find(function(t) { return t.id === id; });
            if (t) t.visible = false;
            var self = this;
            setTimeout(function() {
                self.toasts = self.toasts.filter(function(t) { return t.id !== id; });
            }, 300);
        }
    };
}
