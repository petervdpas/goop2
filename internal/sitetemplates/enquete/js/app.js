// Enquete app.js
//
// The fields array below is the single source of truth.
// goop-form.js will create the table and add any missing columns
// automatically — no separate seed() needed.
(async function () {
  await Goop.form.render(document.getElementById("enquete-form"), {
    table: "responses",
    fields: [
      {
        name: "q1",
        label: "What brings you to this network?",
        type: "text",
        placeholder: "e.g. curiosity, a friend told me, building something...",
      },
      {
        name: "q2",
        label: "What topics interest you most?",
        type: "select",
        options: ["Technology", "Art", "Music", "Science", "Community", "Other"],
      },
      {
        name: "q3",
        label: "Anything else you'd like to share?",
        type: "textarea",
        placeholder: "Feedback, ideas, suggestions...",
      },
    ],
    submitLabel: "Submit",
    singleResponse: true,
  });
})();
