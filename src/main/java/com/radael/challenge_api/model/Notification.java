package com.radael.challenge_api.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

import java.util.Date;
import java.util.Objects;

@Document(collection = "notifications")
public class Notification {
    @Id
    private String id;
    private String content;
    private String groupName;
    private Date sentAt;

    // Construtor padrão
    public Notification() {
    }

    // Construtor com parâmetros
    public Notification(String content, String groupName, Date sentAt) {
        this.content = content;
        this.groupName = groupName;
        this.sentAt = sentAt != null ? new Date(sentAt.getTime()) : null; // Defensiva contra mutabilidade
    }

    // Getters e Setters
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getContent() {
        return content;
    }

    public void setContent(String content) {
        this.content = content;
    }

    public String getGroupName() {
        return groupName;
    }

    public void setGroupName(String groupName) {
        this.groupName = groupName;
    }

    public Date getSentAt() {
        return sentAt != null ? new Date(sentAt.getTime()) : null; // Evita retorno de referência mutável
    }

    public void setSentAt(Date sentAt) {
        this.sentAt = sentAt != null ? new Date(sentAt.getTime()) : null;
    }

    // Método toString para facilitar a exibição dos dados da Notification
    @Override
    public String toString() {
        return "Notification{" +
                "id='" + id + '\'' +
                ", content='" + content + '\'' +
                ", groupName='" + groupName + '\'' +
                ", sentAt=" + sentAt +
                '}';
    }

    // Override do método equals para comparação precisa entre objetos Notification
    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        Notification that = (Notification) o;
        return Objects.equals(id, that.id) &&
               Objects.equals(content, that.content) &&
               Objects.equals(groupName, that.groupName) &&
               Objects.equals(sentAt, that.sentAt);
    }

    // Override do método hashCode para uso eficiente em coleções
    @Override
    public int hashCode() {
        return Objects.hash(id, content, groupName, sentAt);
    }
}
