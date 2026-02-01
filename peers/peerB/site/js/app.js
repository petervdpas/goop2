// Quiz app.js — Full-stack quiz with server-side scoring via Lua
(async function () {
  var db = Goop.data;
  var root = document.getElementById("quiz-root");
  var isOwner = false;

  // Detect owner
  var myId = await Goop.identity.id();
  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  if (!match || match[1] === myId) {
    isOwner = true;
  }

  // Seed sample questions on first run (owner only)
  if (isOwner) {
    await seed();
  }

  if (isOwner) {
    renderOwner();
  } else {
    renderQuiz();
  }

  // ── Seed ──

  async function seed() {
    var tables = await db.tables();
    var hasQuestions = tables && tables.some(function (t) { return t.name === "questions"; });
    if (hasQuestions) {
      var existing = await db.query("questions", { limit: 1 });
      if (existing && existing.length > 0) return;
    }

    // Insert sample questions
    await db.insert("questions", {
      question: "What does HTML stand for?",
      option_a: "Hyper Text Markup Language",
      option_b: "High Tech Modern Language",
      option_c: "Home Tool Markup Language",
      option_d: "Hyperlink and Text Markup Language",
      correct: "a"
    });
    await db.insert("questions", {
      question: "Which protocol does the web primarily use?",
      option_a: "FTP",
      option_b: "SMTP",
      option_c: "HTTP",
      option_d: "SSH",
      correct: "c"
    });
    await db.insert("questions", {
      question: "What does CSS stand for?",
      option_a: "Computer Style Sheets",
      option_b: "Cascading Style Sheets",
      option_c: "Creative Style System",
      option_d: "Colorful Style Sheets",
      correct: "b"
    });
  }

  // ── Owner view ──

  async function renderOwner() {
    var questions = await db.query("questions") || [];
    var scores = await db.query("scores", { limit: 50 }) || [];

    var html = '<div class="qz-manage">';
    html += '<h2>Manage Questions</h2>';

    if (questions.length === 0) {
      html += '<p class="qz-empty">No questions yet.</p>';
    } else {
      html += '<table><thead><tr><th>#</th><th>Question</th><th>Answer</th><th></th></tr></thead><tbody>';
      for (var i = 0; i < questions.length; i++) {
        var q = questions[i];
        html += '<tr>';
        html += '<td>' + (i + 1) + '</td>';
        html += '<td>' + esc(q.question) + '</td>';
        html += '<td>' + esc(q.correct).toUpperCase() + '</td>';
        html += '<td><button class="btn-sm" data-del="' + q._id + '">Delete</button></td>';
        html += '</tr>';
      }
      html += '</tbody></table>';
    }

    html += '<button class="qz-add-btn" id="add-q">+ Add Question</button>';
    html += '</div>';

    // Scores
    html += '<div class="qz-manage qz-scores">';
    html += '<h2>Scores</h2>';
    if (scores.length === 0) {
      html += '<p class="qz-empty">No submissions yet.</p>';
    } else {
      html += '<table><thead><tr><th>Peer</th><th>Score</th><th>Date</th></tr></thead><tbody>';
      for (var j = 0; j < scores.length; j++) {
        var s = scores[j];
        html += '<tr>';
        html += '<td>' + esc(s.peer_label || s._owner.substring(0, 12) + '...') + '</td>';
        html += '<td>' + s.score + '/' + s.total + '</td>';
        html += '<td>' + esc(s._created_at || '') + '</td>';
        html += '</tr>';
      }
      html += '</tbody></table>';
    }
    html += '</div>';

    root.innerHTML = html;

    // Delete handlers
    root.querySelectorAll("[data-del]").forEach(function (btn) {
      btn.onclick = async function () {
        await db.remove("questions", parseInt(btn.getAttribute("data-del")));
        renderOwner();
      };
    });

    // Add handler
    document.getElementById("add-q").onclick = function () { showAddForm(); };
  }

  function showAddForm() {
    var form = document.createElement("div");
    form.className = "qz-card";
    form.innerHTML =
      '<h3>New Question</h3>' +
      '<div style="display:flex;flex-direction:column;gap:0.5rem">' +
        '<input id="nq" placeholder="Question text" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
        '<input id="na" placeholder="Option A" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
        '<input id="nb" placeholder="Option B" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
        '<input id="nc" placeholder="Option C" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
        '<input id="nd" placeholder="Option D" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
        '<select id="ncorrect" style="padding:0.4rem;border:1px solid #d8dce6;border-radius:6px">' +
          '<option value="a">Correct: A</option><option value="b">Correct: B</option>' +
          '<option value="c">Correct: C</option><option value="d">Correct: D</option>' +
        '</select>' +
        '<button class="qz-submit" id="save-q">Save Question</button>' +
      '</div>';
    root.insertBefore(form, root.firstChild);

    document.getElementById("save-q").onclick = async function () {
      var q = document.getElementById("nq").value.trim();
      if (!q) return;
      await db.insert("questions", {
        question: q,
        option_a: document.getElementById("na").value.trim() || "A",
        option_b: document.getElementById("nb").value.trim() || "B",
        option_c: document.getElementById("nc").value.trim() || "C",
        option_d: document.getElementById("nd").value.trim() || "D",
        correct: document.getElementById("ncorrect").value
      });
      renderOwner();
    };
  }

  // ── Visitor view: take quiz ──

  async function renderQuiz() {
    // Check if this peer already submitted
    var scores = await db.query("scores") || [];
    for (var s = 0; s < scores.length; s++) {
      if (scores[s]._owner === myId) {
        showResult(scores[s], true);
        return;
      }
    }

    renderQuizForm();
  }

  async function renderQuizForm() {
    var questions = await db.query("questions") || [];

    if (questions.length === 0) {
      root.innerHTML = '<p class="qz-empty">No questions available yet.</p>';
      return;
    }

    var html = '';
    for (var i = 0; i < questions.length; i++) {
      var q = questions[i];
      html += '<div class="qz-card">';
      html += '<h3><span class="qz-num">' + (i + 1) + '.</span> ' + esc(q.question) + '</h3>';
      html += '<div class="qz-options">';
      var opts = ["a", "b", "c", "d"];
      for (var j = 0; j < opts.length; j++) {
        var key = opts[j];
        var text = q["option_" + key];
        html += '<div class="qz-option">';
        html += '<input type="radio" name="q' + q._id + '" id="q' + q._id + key + '" value="' + key + '">';
        html += '<label for="q' + q._id + key + '">' + esc(text) + '</label>';
        html += '</div>';
      }
      html += '</div></div>';
    }

    html += '<button class="qz-submit" id="submit-quiz">Submit Answers</button>';
    root.innerHTML = html;

    document.getElementById("submit-quiz").onclick = async function () {
      var btn = this;
      btn.disabled = true;
      btn.textContent = "Scoring...";

      // Collect answers
      var answers = {};
      for (var i = 0; i < questions.length; i++) {
        var sel = root.querySelector('input[name="q' + questions[i]._id + '"]:checked');
        if (sel) {
          answers[String(questions[i]._id)] = sel.value;
        }
      }

      try {
        // Call server-side scoring function
        var result = await db.call("score", { answers: answers });
        showResult(result, true);
      } catch (err) {
        root.innerHTML = '<div class="qz-result"><p class="msg">Error: ' + esc(err.message || String(err)) + '</p></div>';
      }
    };
  }

  function showResult(r, allowRetake) {
    var pct = r.total > 0 ? Math.round((r.score / r.total) * 100) : 0;
    var passed = r.passed !== undefined ? r.passed : r.score >= Math.ceil(r.total * 0.7);
    var cls = passed ? "pass" : "fail";
    var emoji = passed ? "🎉" : "😔";
    var msg = r.message || (r.score + " out of " + r.total + " correct");

    var html =
      '<div class="qz-result">' +
        '<div class="score ' + cls + '">' + r.score + ' / ' + r.total + '</div>' +
        '<div class="label">' + pct + '% correct</div>' +
        '<div class="msg">' + emoji + ' ' + esc(msg) + '</div>' +
        (allowRetake ? '<button class="qz-submit" id="retake-quiz" style="margin-top:1.5rem;">Retake Quiz</button>' : '') +
      '</div>';
    root.innerHTML = html;

    if (allowRetake) {
      document.getElementById("retake-quiz").onclick = function () {
        renderQuizForm();
      };
    }
  }

  function esc(s) {
    if (!s) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
