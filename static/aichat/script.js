const userInput = document.getElementById('user-input');
const sendButton = document.getElementById('send-button');
const chatBox = document.querySelector('.chat-box');

sendButton.addEventListener('click', () => {
    const userInputValue = userInput.value.trim();
    if (!userInputValue) {
        return;
    }

    // 将用户输入的内容添加到聊天框中
    const userMessage = document.createElement('div');
    userMessage.classList.add('user-message');
    userMessage.innerHTML = `<p>${userInputValue}</p>`;
    chatBox.appendChild(userMessage);

    // 清空用户输入框
    userInput.value = '';

    // 模拟AI响应
    const aiResponse = '您的问题或要求已收到，AI正在处理中，请稍候。';
    const aiMessage = document.createElement('div');
    aiMessage.classList.add('ai-message');
    aiMessage.innerHTML = `<p>${aiResponse}</p>`;
    chatBox.appendChild(aiMessage);

    // 模拟AI响应延迟
    setTimeout(() => {
        const aiResponse = '以下是AI对您的问题的回答或对您要求的处理结果：';
        const aiMessage = document.createElement('div');
        aiMessage.classList.add('ai-message');
        aiMessage.innerHTML = `<p>${aiResponse}</p>`;
        chatBox.appendChild(aiMessage);
    }, 1000);
});
