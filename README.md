<div style="text-align: center;">
    <img src="assets/condoguard-logo.svg" alt="CondoguardLogo" width="200" height="200">
</div>


# CondoGuard

CondoGuard é um aplicativo em desenvolvimento que visa ajudar os condôminos a administrar suas despesas condominiais de forma eficiente e preventiva. Com uma abordagem inovadora, o CondoGuard permite que os usuários gerenciem suas despesas, façam previsões financeiras e identifiquem possíveis problemas antes que eles se tornem críticos.

## Objetivo

O objetivo principal do CondoGuard é fornecer uma ferramenta robusta e amigável para a gestão financeira de condomínios, ajudando tanto os administradores quanto os moradores a terem uma visão clara de suas despesas, além de se prevenirem contra futuros problemas com base no histórico de gastos.

## Funcionalidades

- **Gerenciamento de Despesas**: Registre e acompanhe todas as despesas do condomínio em tempo real.
- **Previsão de Gastos**: Use dados históricos para prever gastos futuros e planejar o orçamento.
- **Notificações Inteligentes**: Receba alertas sobre possíveis problemas, como vazamentos ou aumentos inesperados de consumo.
- **Autenticação Segura**: Sistema de login seguro utilizando JWT para proteger dados sensíveis.
- **Sistema de Comunicação**: Integração para permitir uma comunicação eficaz entre síndicos e moradores.

## Instalação e Configuração

1. Clone o repositório:

    ```bash
    git clone https://github.com/seu-usuario/condoguard.git
    cd condoguard
    ```

2. Certifique-se de ter o MongoDB em execução localmente ou configure a URL de conexão no arquivo `application.properties`:

    ```properties
    spring.data.mongodb.uri=mongodb://localhost:27017/condoguard
    ```

3. Configure o segredo JWT no arquivo `application.properties`:

    ```properties
    jwt.secret=SeuSegredoJWT
    ```

4. Execute o projeto:

    ```bash
    mvn spring-boot:run
    ```

## Endpoints da API

### Autenticação

- **POST** `/auth/register`: Registrar um novo usuário.
- **POST** `/auth/login`: Fazer login com credenciais e receber um token JWT.

### Usuários

- **GET** `/users`: Listar todos os usuários (requer autenticação).
- **GET** `/users/{id}`: Obter detalhes de um usuário específico (requer autenticação).

### Despesas

- **GET** `/expenses`: Listar todas as despesas (requer autenticação).
- **POST** `/expenses`: Criar uma nova despesa (requer autenticação).
- **PUT** `/expenses/{id}`: Atualizar uma despesa existente (requer autenticação).
- **DELETE** `/expenses/{id}`: Excluir uma despesa (requer autenticação).

## Contribuição

Contribuições são bem-vindas! Se você deseja adicionar novas funcionalidades, corrigir bugs ou melhorar a documentação, siga os passos abaixo:

1. Faça um fork do projeto.
2. Crie uma nova branch para a sua feature:

    ```bash
    git checkout -b feature/nova-feature
    ```

3. Faça as alterações desejadas e commit:

    ```bash
    git commit -m "Adicionar nova feature"
    ```

4. Envie suas alterações para o repositório:

    ```bash
    git push origin feature/nova-feature
    ```

5. Abra um Pull Request explicando as mudanças propostas.

## Futuras Melhorias

CondoGuard está em constante desenvolvimento. Algumas das funcionalidades planejadas para as próximas versões incluem:

- **Integração com Sistemas de Pagamento**: Permitir que os usuários paguem suas despesas diretamente pelo aplicativo.
- **Dashboard Analítico**: Visualize dados financeiros e estatísticas de consumo.
- **Integração com IoT**: Monitore consumo de energia, água e gás em tempo real.

## Licença

Este projeto é licenciado sob a [GNU General Public License v3.0](LICENSE).

## Contato

Para mais informações ou sugestões, entre em contato conosco pelo e-mail: [radael.engenharia@gmail.com](mailto:seu-email@exemplo.com).

---

**CondoGuard** - Simplificando a gestão do seu condomínio!
