// Quiz app.js
(async function () {
  var h = Goop.dom;
  var db = Goop.data;
  var ctx = await Goop.peer();
  var questions = await db.orm("questions");
  var scores = await db.orm("scores");
  var root = document.getElementById("quiz-root");

  if (ctx.isOwner) { await seed(); renderOwner(); }
  else { renderQuiz(); }

  async function seed() {
    var existing = await questions.find({ limit: 1 });
    if (existing && existing.length > 0) return;
    await questions.insert({ question: "What does HTML stand for?", option_a: "Hyper Text Markup Language", option_b: "High Tech Modern Language", option_c: "Home Tool Markup Language", option_d: "Hyperlink and Text Markup Language", correct: "a" });
    await questions.insert({ question: "Which protocol does the web primarily use?", option_a: "FTP", option_b: "SMTP", option_c: "HTTP", option_d: "SSH", correct: "c" });
    await questions.insert({ question: "What does CSS stand for?", option_a: "Computer Style Sheets", option_b: "Cascading Style Sheets", option_c: "Creative Style System", option_d: "Colorful Style Sheets", correct: "b" });
  }

  async function renderOwner() {
    var qs = await questions.find() || [];
    var sc = await scores.find({ limit: 50 }) || [];

    Goop.render(root,
      h("div", { class: "qz-manage" },
        h("h2", {}, "Manage Questions"),
        qs.length === 0
          ? h("p", { class: "qz-empty" }, "No questions yet.")
          : h("table", {},
              h("thead", {}, h("tr", {}, h("th", {}, "#"), h("th", {}, "Question"), h("th", {}, "Answer"), h("th", {}))),
              h("tbody", {}, qs.map(function(q, i) {
                return h("tr", {},
                  h("td", {}, String(i + 1)),
                  h("td", {}, q.question),
                  h("td", {}, q.correct.toUpperCase()),
                  h("td", {}, h("button", { class: "btn-sm", onclick: async function() { await questions.remove(q._id); renderOwner(); } }, "Delete"))
                );
              }))
            ),
        h("button", { class: "qz-add-btn", onclick: showAddForm }, "+ Add Question")
      ),
      h("div", { class: "qz-manage qz-scores" },
        h("h2", {}, "Scores"),
        sc.length === 0
          ? h("p", { class: "qz-empty" }, "No submissions yet.")
          : h("table", {},
              h("thead", {}, h("tr", {}, h("th", {}, "Peer"), h("th", {}, "Score"), h("th", {}, "Date"))),
              h("tbody", {}, sc.map(function(s) {
                return h("tr", {},
                  h("td", {}, s.peer_label || s._owner.substring(0, 12) + "..."),
                  h("td", {}, s.score + "/" + s.total),
                  h("td", {}, s._created_at || "")
                );
              }))
            )
      )
    );
  }

  function showAddForm() {
    var form = h("div", { class: "qz-card qz-add-form" },
      h("h3", {}, "New Question"),
      h("div", { class: "qz-field" },
        h("label", {}, "Question"),
        h("textarea", { id: "nq", class: "qz-input qz-textarea", placeholder: "Type your question here...", rows: "2" })
      ),
      h("div", { class: "qz-options-grid" },
        ["A", "B", "C", "D"].map(function(l, i) {
          var v = l.toLowerCase();
          return h("div", { class: "qz-option-field" },
            h("label", {}, h("span", { class: "qz-option-letter" }, l)),
            h("input", { id: "n" + v, class: "qz-input", placeholder: "Option " + l }),
            h("input", { type: "radio", name: "ncorrect", value: v, class: "qz-correct-radio", checked: i === 0 ? "checked" : null })
          );
        })
      ),
      h("div", { class: "qz-form-hint" }, "Select the radio button next to the correct answer."),
      h("div", { class: "qz-form-actions" },
        h("button", { class: "qz-submit", onclick: async function() {
          var q = document.getElementById("nq").value.trim();
          if (!q) return;
          var correct = root.querySelector('input[name="ncorrect"]:checked');
          await questions.insert({
            question: q,
            option_a: document.getElementById("na").value.trim() || "Option A",
            option_b: document.getElementById("nb").value.trim() || "Option B",
            option_c: document.getElementById("nc").value.trim() || "Option C",
            option_d: document.getElementById("nd").value.trim() || "Option D",
            correct: correct ? correct.value : "a",
          });
          renderOwner();
        } }, "Save Question"),
        h("button", { class: "qz-cancel", onclick: function() { form.remove(); } }, "Cancel")
      )
    );
    root.insertBefore(form, root.firstChild);
  }

  async function renderQuiz() {
    var sc = await scores.find() || [];
    for (var s = 0; s < sc.length; s++) {
      if (sc[s]._owner === ctx.myId) { showResult(sc[s]); return; }
    }
    renderQuizForm();
  }

  async function renderQuizForm() {
    var qs = await questions.find() || [];
    if (qs.length === 0) { Goop.render(root, h("p", { class: "qz-empty" }, "No questions available yet.")); return; }

    var qData = qs.map(function(q, i) { return Object.assign({ num: i + 1 }, q); });
    root.innerHTML = "";
    for (var qi = 0; qi < qData.length; qi++) {
      root.appendChild(await Goop.partial("question-card", qData[qi]));
    }
    root.appendChild(h("button", { class: "qz-submit", onclick: async function() {
        this.disabled = true; this.textContent = "Scoring...";
        var answers = {};
        for (var i = 0; i < qs.length; i++) {
          var sel = root.querySelector('input[name="q' + qs[i]._id + '"]:checked');
          if (sel) answers[String(qs[i]._id)] = sel.value;
        }
        try { showResult(await db.call("score", { answers: answers })); }
        catch (err) { Goop.render(root, h("p", { class: "qz-empty" }, "Error: " + err.message)); }
      } }, "Submit Answers")
    );
  }

  function showResult(r) {
    var pct = r.total > 0 ? Math.round((r.score / r.total) * 100) : 0;
    var passed = r.passed !== undefined ? r.passed : r.score >= Math.ceil(r.total * 0.7);
    Goop.render(root,
      h("div", { class: "qz-result" },
        h("div", { class: "score " + (passed ? "pass" : "fail") }, r.score + " / " + r.total),
        h("div", { class: "label" }, pct + "% correct"),
        h("div", { class: "msg" }, r.message || r.score + " out of " + r.total + " correct"),
        h("button", { class: "qz-submit", onclick: function() { renderQuizForm(); } }, "Retake Quiz")
      )
    );
  }
})();
