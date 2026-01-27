/* internal/ui/assets/js/99-toast.js */

// Toast notification system
(function() {
  let toastContainer;

  function ensureContainer() {
    if (!toastContainer) {
      toastContainer = document.createElement('div');
      toastContainer.className = 'toast-container';
      document.body.appendChild(toastContainer);
    }
    return toastContainer;
  }

  function showToast(options) {
    const container = ensureContainer();
    
    const toast = document.createElement('div');
    toast.className = 'toast';
    
    const icon = options.icon || '💬';
    const title = options.title || 'Notification';
    const message = options.message || '';
    const onClick = options.onClick || null;
    const duration = options.duration || 5000;

    toast.innerHTML = `
      <div class="toast-header">
        <span class="toast-icon">${icon}</span>
        <span class="toast-title">${title}</span>
        <button class="toast-close" aria-label="Close">×</button>
      </div>
      <div class="toast-body">${message}</div>
    `;

    container.appendChild(toast);

    // Close button
    const closeBtn = toast.querySelector('.toast-close');
    closeBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      removeToast(toast);
    });

    // Click handler
    if (onClick) {
      toast.style.cursor = 'pointer';
      toast.addEventListener('click', () => {
        onClick();
        removeToast(toast);
      });
    }

    // Auto-remove after duration
    if (duration > 0) {
      setTimeout(() => {
        removeToast(toast);
      }, duration);
    }

    return toast;
  }

  function removeToast(toast) {
    toast.classList.add('toast-exit');
    setTimeout(() => {
      if (toast.parentNode) {
        toast.parentNode.removeChild(toast);
      }
    }, 300);
  }

  // Export to window
  window.Goop = window.Goop || {};
  window.Goop.toast = showToast;
})();
