const userInput = document.getElementById('userInput');
const submitButton = document.getElementById('submitButton');
const aiResponse = document.querySelector('.ai-response');

submitButton.addEventListener('click', () => {
  const userQuestion = userInput.value;
  aiResponse.innerHTML = `<p>${userQuestion}</p>`;
  userInput.value = '';

  // 此处需要编写与AI交互的代码，将用户输入的问题发送给AI并获取回复
  // 将AI的回复显示在 aiResponse 中
});
