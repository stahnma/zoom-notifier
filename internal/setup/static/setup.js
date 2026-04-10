// zoom-notifier setup wizard scripts

document.addEventListener("DOMContentLoaded", function () {
    // Regenerate API key button
    var regenBtn = document.getElementById("regenerate-key-btn");
    if (regenBtn) {
        regenBtn.addEventListener("click", function () {
            fetch("/setup/api/generate-key")
                .then(function (resp) {
                    return resp.json();
                })
                .then(function (data) {
                    if (data.key) {
                        document.getElementById("admin_api_key").value = data.key;
                    }
                })
                .catch(function (err) {
                    console.error("Failed to regenerate key:", err);
                });
        });
    }

    // Show/hide secret toggle buttons
    var toggleButtons = document.querySelectorAll(".toggle-secret");
    toggleButtons.forEach(function (btn) {
        btn.addEventListener("click", function () {
            var span = btn.previousElementSibling;
            if (!span || !span.classList.contains("secret-value")) return;

            var secret = span.getAttribute("data-secret");
            if (btn.textContent === "Show") {
                span.textContent = secret;
                btn.textContent = "Hide";
            } else {
                span.textContent = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022";
                btn.textContent = "Show";
            }
        });
    });
});

// Copy text to clipboard with toast feedback
function copyToClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function () {
            showToast("Copied!");
        });
    } else {
        // Fallback for older browsers
        var textarea = document.createElement("textarea");
        textarea.value = text;
        textarea.style.position = "fixed";
        textarea.style.opacity = "0";
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand("copy");
        document.body.removeChild(textarea);
        showToast("Copied!");
    }
}

function showToast(message) {
    var existing = document.querySelector(".toast");
    if (existing) existing.remove();

    var toast = document.createElement("div");
    toast.className = "toast";
    toast.textContent = message;
    document.body.appendChild(toast);

    // Trigger reflow then show
    toast.offsetHeight;
    toast.classList.add("visible");

    setTimeout(function () {
        toast.classList.remove("visible");
        setTimeout(function () {
            toast.remove();
        }, 200);
    }, 1500);
}
