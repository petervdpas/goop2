// goop-emoji.js — lightweight emoji picker for chat inputs
(function() {
  var emojis = {
    'Smileys': [
      '\u{1F600}','\u{1F603}','\u{1F604}','\u{1F601}','\u{1F605}','\u{1F602}','\u{1F923}',
      '\u{1F60A}','\u{1F607}','\u{1F642}','\u{1F643}','\u{1F609}','\u{1F60C}','\u{1F60D}',
      '\u{1F618}','\u{1F617}','\u{1F61A}','\u{1F60B}','\u{1F61C}','\u{1F61D}','\u{1F61B}',
      '\u{1F911}','\u{1F917}','\u{1F914}','\u{1F910}','\u{1F928}','\u{1F610}','\u{1F611}',
      '\u{1F636}','\u{1F60F}','\u{1F612}','\u{1F644}','\u{1F62C}','\u{1F925}','\u{1F60E}',
      '\u{1F634}','\u{1F62A}','\u{1F922}','\u{1F92E}','\u{1F927}','\u{1F975}','\u{1F976}'
    ],
    'Gestures': [
      '\u{1F44D}','\u{1F44E}','\u{1F44F}','\u{1F64C}','\u{1F91D}','\u{1F64F}','\u{270D}\uFE0F',
      '\u{1F4AA}','\u{1F448}','\u{1F449}','\u{1F446}','\u{1F447}','\u{270C}\uFE0F','\u{1F91E}',
      '\u{1F919}','\u{1F918}','\u{1F44C}','\u{1F44A}','\u{1F91B}','\u{1F91C}','\u{1F44B}',
      '\u{1F590}\uFE0F','\u{270B}','\u{1F596}'
    ],
    'Hearts': [
      '\u2764\uFE0F','\u{1F9E1}','\u{1F49B}','\u{1F49A}','\u{1F499}','\u{1F49C}','\u{1F5A4}',
      '\u{1F90D}','\u{1F90E}','\u{1F498}','\u{1F49D}','\u{1F496}','\u{1F497}','\u{1F493}',
      '\u{1F49E}','\u{1F495}','\u{1F48C}','\u{1F49F}'
    ],
    'Things': [
      '\u{1F389}','\u{1F388}','\u{1F381}','\u{1F386}','\u{1F387}','\u{1F390}','\u{1F3B5}',
      '\u{1F3B6}','\u{1F3B8}','\u{1F3AE}','\u{1F3AF}','\u{1F525}','\u2B50','\u{1F31F}',
      '\u26A1','\u{1F4A5}','\u{1F4AB}','\u{1F4A8}','\u{1F308}','\u2600\uFE0F','\u{1F326}\uFE0F',
      '\u2601\uFE0F','\u{1F4BB}','\u{1F4F1}','\u{1F512}','\u{1F513}','\u{1F4E7}','\u2705',
      '\u274C','\u2753','\u{1F4A1}','\u{1F680}','\u{1F6A8}'
    ],
    'Animals': [
      '\u{1F436}','\u{1F431}','\u{1F42D}','\u{1F439}','\u{1F430}','\u{1F98A}','\u{1F43B}',
      '\u{1F43C}','\u{1F428}','\u{1F42F}','\u{1F981}','\u{1F42E}','\u{1F437}','\u{1F438}',
      '\u{1F435}','\u{1F412}','\u{1F414}','\u{1F427}','\u{1F426}','\u{1F985}','\u{1F41D}',
      '\u{1F98B}','\u{1F40C}','\u{1F422}'
    ],
    'Food': [
      '\u{1F34E}','\u{1F34A}','\u{1F34B}','\u{1F34C}','\u{1F349}','\u{1F347}','\u{1F353}',
      '\u{1F352}','\u{1F351}','\u{1F34D}','\u{1F354}','\u{1F355}','\u{1F32E}','\u{1F32F}',
      '\u{1F37F}','\u{1F366}','\u{1F370}','\u{1F382}','\u{1F36B}','\u{1F369}','\u2615',
      '\u{1F37A}','\u{1F37B}','\u{1F942}'
    ]
  };

  // Category icons for tabs
  var categoryIcons = {
    'Smileys': '\u{1F600}',
    'Gestures': '\u{1F44D}',
    'Hearts': '\u2764\uFE0F',
    'Things': '\u2B50',
    'Animals': '\u{1F436}',
    'Food': '\u{1F354}'
  };

  /**
   * Create an emoji picker and attach it to a chat input form.
   * @param {HTMLFormElement} form - The form element containing the input
   * @param {HTMLInputElement} input - The text input to insert emojis into
   */
  function attachEmojiPicker(form, input) {
    if (!form || !input) return;

    // Create the emoji toggle button
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'emoji-btn';
    btn.title = 'Emoji';
    btn.textContent = '\u{1F642}';
    btn.setAttribute('aria-label', 'Open emoji picker');

    // Insert button before the Send button
    var sendBtn = form.querySelector('button[type="submit"]');
    if (sendBtn) {
      form.insertBefore(btn, sendBtn);
    } else {
      form.appendChild(btn);
    }

    // Create picker panel
    var picker = document.createElement('div');
    picker.className = 'emoji-picker';
    picker.style.display = 'none';

    // Build category tabs
    var tabs = document.createElement('div');
    tabs.className = 'emoji-tabs';
    var categories = Object.keys(emojis);
    var grid = document.createElement('div');
    grid.className = 'emoji-grid';

    categories.forEach(function(cat, idx) {
      var tab = document.createElement('button');
      tab.type = 'button';
      tab.className = 'emoji-tab' + (idx === 0 ? ' active' : '');
      tab.textContent = categoryIcons[cat] || cat.charAt(0);
      tab.title = cat;
      tab.setAttribute('data-cat', cat);
      tab.addEventListener('click', function(e) {
        e.stopPropagation();
        tabs.querySelectorAll('.emoji-tab').forEach(function(t) { t.classList.remove('active'); });
        tab.classList.add('active');
        renderGrid(cat);
      });
      tabs.appendChild(tab);
    });

    picker.appendChild(tabs);
    picker.appendChild(grid);

    // Position picker in the form (above the input bar)
    form.style.position = 'relative';
    form.appendChild(picker);

    function renderGrid(cat) {
      var list = emojis[cat] || [];
      grid.innerHTML = '';
      list.forEach(function(emoji) {
        var span = document.createElement('span');
        span.className = 'emoji-item';
        span.textContent = emoji;
        span.addEventListener('click', function(e) {
          e.stopPropagation();
          insertAtCursor(input, emoji);
          input.focus();
        });
        grid.appendChild(span);
      });
    }

    // Initial render
    renderGrid(categories[0]);

    // Toggle picker
    btn.addEventListener('click', function(e) {
      e.stopPropagation();
      var visible = picker.style.display !== 'none';
      picker.style.display = visible ? 'none' : '';
      btn.classList.toggle('active', !visible);
    });

    // Close picker on outside click
    document.addEventListener('click', function(e) {
      if (!picker.contains(e.target) && e.target !== btn) {
        picker.style.display = 'none';
        btn.classList.remove('active');
      }
    });

    // Close picker on form submit
    form.addEventListener('submit', function() {
      picker.style.display = 'none';
      btn.classList.remove('active');
    });

    // Live shortcode → emoji as user types
    input.addEventListener('input', function() {
      liveEmojify(input);
    });
  }

  function insertAtCursor(input, text) {
    var start = input.selectionStart;
    var end = input.selectionEnd;
    var val = input.value;
    input.value = val.substring(0, start) + text + val.substring(end);
    var pos = start + text.length;
    input.setSelectionRange(pos, pos);
    // Trigger input event so any listeners see the change
    input.dispatchEvent(new Event('input', { bubbles: true }));
  }

  // ── Text shortcode → emoji conversion ──
  // Longest codes first so e.g. "O:)" matches before ":)"
  var shortcodeList = [
    [':thumbsup:', '\u{1F44D}'],
    [':rocket:',   '\u{1F680}'],
    [':heart:',    '\u2764\uFE0F'],
    [':check:',    '\u2705'],
    [':fire:',     '\u{1F525}'],
    [':wave:',     '\u{1F44B}'],
    [':clap:',     '\u{1F44F}'],
    [':star:',     '\u2B50'],
    [':100:',      '\u{1F4AF}'],
    [':x:',        '\u274C'],
    [":'(",        '\u{1F622}'],
    ['O:)',        '\u{1F607}'],
    ['o:)',        '\u{1F607}'],
    [':)',         '\u{1F642}'],
    [':(',         '\u{1F61E}'],
    [':D',         '\u{1F601}'],
    [';)',         '\u{1F609}'],
    [':P',         '\u{1F61B}'],
    [':p',         '\u{1F61B}'],
    [':O',         '\u{1F62E}'],
    [':o',         '\u{1F62E}'],
    ['XD',         '\u{1F606}'],
    ['xD',         '\u{1F606}'],
    ['B)',         '\u{1F60E}'],
    [':/',         '\u{1F615}'],
    [':*',         '\u{1F618}'],
    ['<3',         '\u2764\uFE0F'],
  ];

  /**
   * Replace ALL shortcodes in a string (for rendering stored messages).
   */
  function emojify(text) {
    if (!text) return text;
    for (var i = 0; i < shortcodeList.length; i++) {
      var code = shortcodeList[i][0];
      var emoji = shortcodeList[i][1];
      // Global replace — split/join is simplest for literal strings
      while (text.indexOf(code) !== -1) {
        text = text.replace(code, emoji);
      }
    }
    return text;
  }

  /**
   * Live-replace: check if text just before cursor ends with a shortcode.
   * Replaces in-place and adjusts cursor. Called on every keystroke.
   */
  function liveEmojify(input) {
    var val = input.value;
    var cursor = input.selectionStart;
    var before = val.substring(0, cursor);

    for (var i = 0; i < shortcodeList.length; i++) {
      var code = shortcodeList[i][0];
      var emoji = shortcodeList[i][1];
      if (before.length >= code.length && before.substring(before.length - code.length) === code) {
        var after = val.substring(cursor);
        input.value = before.substring(0, before.length - code.length) + emoji + after;
        var newPos = cursor - code.length + emoji.length;
        input.setSelectionRange(newPos, newPos);
        return;
      }
    }
  }

  /**
   * Check if a string contains only emoji characters (and whitespace).
   * Used to decide whether to render a message in "big emoji" style.
   */
  var emojiOnlyRe = /^[\s\u{FE0F}\u{200D}\u{20E3}\u{1F3FB}-\u{1F3FF}\u{E0020}-\u{E007F}\u{E0001}\u{200B}\u{2000}-\u{200F}\u{2028}-\u{202F}\u{2060}-\u{206F}\u{2100}-\u{27BF}\u{2934}-\u{2935}\u{2B05}-\u{2B07}\u{2B1B}-\u{2B1C}\u{2B50}\u{2B55}\u{3030}\u{303D}\u{3297}\u{3299}\u{2300}-\u{23FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{2764}\u{1F000}-\u{1FAFF}\u{E0000}-\u{E007F}]+$/u;

  function isEmojiOnly(text) {
    if (!text || !text.trim()) return false;
    return emojiOnlyRe.test(text);
  }

  // Expose globally
  window.Goop = window.Goop || {};
  window.Goop.emoji = { attach: attachEmojiPicker, emojify: emojify, isEmojiOnly: isEmojiOnly };

  // Auto-attach to all .chat-input forms on the page
  document.querySelectorAll('form.chat-input').forEach(function(form) {
    var input = form.querySelector('input[type="text"]');
    if (input) attachEmojiPicker(form, input);
  });
})();
