/*
 * This file is part of CondoGuard.
 *
 * CondoGuard is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * CondoGuard is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with CondoGuard. If not, see <https://www.gnu.org/licenses/>.
 */

package com.radael.condoguard.model;

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
